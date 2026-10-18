package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func isolateCargoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CARGO_HOME", "")
}

func setupCargoScenarioDir(t *testing.T, files map[string]string) string {
	t.Helper()
	origDir, err := os.Getwd()
	test.Ok(t, err)

	tmpDir, err := os.MkdirTemp("", "drone-cache-cargo-scenario-*")
	test.Ok(t, err)

	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)
		test.Ok(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		test.Ok(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	test.Ok(t, os.Chdir(tmpDir))
	canonicalDir, err := os.Getwd()
	test.Ok(t, err)

	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		_ = os.RemoveAll(tmpDir)
	})

	return canonicalDir
}

// Scenario 1: Single crate at the repository root.
func TestScenario_Cargo_RootProject(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n[[package]]\nname = \"demo\"\nversion = \"0.1.0\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	expected := []string{
		filepath.Join(tmpDir, ".cache", "cargo", "registry", "cache"),
		filepath.Join(tmpDir, ".cache", "cargo", "registry", "index"),
		filepath.Join(tmpDir, ".cache", "cargo", "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

// Scenario 2: Crate one directory level down.
func TestScenario_Cargo_NestedProject(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"service/Cargo.toml": "[package]\nname = \"svc\"\nversion = \"0.1.0\"\n",
		"service/Cargo.lock": "# v1\n[[package]]\nname = \"svc\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	expected := []string{
		filepath.Join(tmpDir, "service", ".cache", "cargo", "registry", "cache"),
		filepath.Join(tmpDir, "service", ".cache", "cargo", "registry", "index"),
		filepath.Join(tmpDir, "service", ".cache", "cargo", "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

// Scenario 3: CARGO_HOME override wins over the repo-local default.
func TestScenario_Cargo_EnvOverride(t *testing.T) {
	isolateCargoEnv(t)
	t.Setenv("CARGO_HOME", ".cargo-home")
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"demo\"\n",
		"Cargo.lock": "# v1\n",
	})

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	expected := []string{
		filepath.Join(tmpDir, ".cargo-home", "registry", "cache"),
		filepath.Join(tmpDir, ".cargo-home", "registry", "index"),
		filepath.Join(tmpDir, ".cargo-home", "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

// Scenario 4: Key invalidation when Cargo.lock changes.
func TestScenario_Cargo_KeyInvalidation(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"demo\"\n",
		"Cargo.lock": "# v1\n[[package]]\nname = \"foo\"\nversion = \"1.0.0\"\n",
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	err = os.WriteFile("Cargo.lock", []byte("# v1\n[[package]]\nname = \"foo\"\nversion = \"2.0.0\"\n"), 0644)
	test.Ok(t, err)

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, hash1 != hash2, "expected key invalidation when Cargo.lock changes")
}

// Scenario 5: Hybrid Rust + Node repo detects both managers.
func TestScenario_Cargo_Hybrid_WithNode(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml":        "[package]\nname = \"demo\"\n",
		"Cargo.lock":        "# v1\n",
		"package.json":      `{"name": "frontend"}`,
		"package-lock.json": `{"name": "frontend", "lockfileVersion": 2}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"node", "cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty composite hash")
	test.Assert(t, cargoContains(dirs, filepath.Join(tmpDir, "node_modules")), "expected node_modules")
	test.Assert(t, cargoContains(dirs, filepath.Join(tmpDir, ".cache", "cargo", "registry", "cache")),
		"expected cargo registry cache")
}

// Scenario 6: skipPrepare yields nothing.
func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"demo\"\n",
		"Cargo.lock": "# v1\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)

	test.Assert(t, len(dirs) == 0, "expected no dirs when skipPrepare is true")
	test.Assert(t, len(tools) == 0, "expected no tools when skipPrepare is true")
	test.Assert(t, hash == "", "expected empty hash when skipPrepare is true")
}

// Scenario 7: Cargo.toml without a lockfile is not detected (lockfile-only).
func TestScenario_Cargo_ManifestWithoutLock(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"demo\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, !cargoContains(tools, "cargo"), "expected no cargo detection without Cargo.lock")
	test.Assert(t, len(dirs) == 0, "expected no dirs")
	test.Assert(t, hash == "", "expected empty hash")
}

// Scenario 8: detection and preparation are deterministic across runs.
func TestScenario_Cargo_Idempotency(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"demo\"\n",
		"Cargo.lock": "# v1\n",
	})

	dirs1, tools1, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	dirs2, tools2, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, dirs1, dirs2)
	test.Equals(t, tools1, tools2)
	test.Equals(t, hash1, hash2)
}

func cargoContains(slice []string, elem string) bool {
	for _, v := range slice {
		if v == elem {
			return true
		}
	}
	return false
}
