package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

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

func cargoExpectedDirs(root string) []string {
	base := filepath.Join(root, ".cache", "cargo")
	return []string{
		filepath.Join(base, "registry", "cache"),
		filepath.Join(base, "registry", "index"),
		filepath.Join(base, "git", "db"),
	}
}

// Scenario 1: Standard binary crate with lockfile and manifest.
func TestScenario_Cargo_WithLockfile(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "[[package]]\nname = \"app\"\nversion = \"0.1.0\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, cargoExpectedDirs(tmpDir), dirs)
}

// Scenario 2: Library crate with manifest only (no Cargo.lock).
func TestScenario_Cargo_FallbackManifestOnly(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"lib\"\nversion = \"0.1.0\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, cargoExpectedDirs(tmpDir), dirs)
}

// Scenario 3: Nested crate (one directory level down).
func TestScenario_Cargo_NestedProject(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"service/Cargo.toml": "[package]\nname = \"svc\"\n",
		"service/Cargo.lock": "[[package]]\nname = \"svc\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, cargoExpectedDirs(filepath.Join(tmpDir, "service")), dirs)
}

// Scenario 4: Lockfile precedence over the manifest fallback (single tool entry).
func TestScenario_Cargo_LockfilePrecedence(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\n",
		"Cargo.lock": "[[package]]\nname = \"app\"\n",
	})

	_, tools, lockHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"cargo"}, tools)

	// The key must come from Cargo.lock, not Cargo.toml.
	manifestHash, _, err := calculateMd5FromFiles([]string{filepath.Join(tmpDir, "Cargo.lock")})
	test.Ok(t, err)
	test.Equals(t, manifestHash, lockHash)
}

// Scenario 5: CARGO_HOME environment override.
func TestScenario_Cargo_EnvVar_CargoHome(t *testing.T) {
	isolateCargoEnv(t)
	t.Setenv("CARGO_HOME", ".cargo-home")
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\n",
		"Cargo.lock": "[[package]]\nname = \"app\"\n",
	})

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	base := filepath.Join(tmpDir, ".cargo-home")
	expected := []string{
		filepath.Join(base, "registry", "cache"),
		filepath.Join(base, "registry", "index"),
		filepath.Join(base, "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

// Scenario 6: Hybrid repo (Rust + Node) detects both managers.
func TestScenario_Cargo_Hybrid_WithNode(t *testing.T) {
	isolateCargoEnv(t)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml":        "[package]\nname = \"app\"\n",
		"Cargo.lock":        "[[package]]\nname = \"app\"\n",
		"package.json":      `{"name": "frontend"}`,
		"package-lock.json": `{"name": "frontend", "lockfileVersion": 2}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"node", "cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty composite hash")

	expected := append([]string{filepath.Join(tmpDir, "node_modules")}, cargoExpectedDirs(tmpDir)...)
	test.Equals(t, expected, dirs)
}

// Scenario 7: Key invalidation on Cargo.lock change.
func TestScenario_Cargo_KeyInvalidation(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\n",
		"Cargo.lock": "[[package]]\nname = \"foo\"\nversion = \"1.0.0\"\n",
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	err = os.WriteFile("Cargo.lock", []byte("[[package]]\nname = \"foo\"\nversion = \"2.0.0\"\n"), 0644)
	test.Ok(t, err)

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, hash1 != hash2, "expected key invalidation when Cargo.lock changes")
}

// Scenario 8: Idempotency and zero mutation of tracked files.
func TestScenario_Cargo_Idempotency(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\n",
		"Cargo.lock": "[[package]]\nname = \"app\"\n",
	})

	before, err := filepath.Glob(filepath.Join(tmpDir, "*"))
	test.Ok(t, err)

	dirs1, tools1, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	dirs2, tools2, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, dirs1, dirs2)
	test.Equals(t, tools1, tools2)
	test.Equals(t, hash1, hash2)

	after, err := filepath.Glob(filepath.Join(tmpDir, "*"))
	test.Ok(t, err)
	test.Equals(t, before, after)
}

// Scenario 9: Skip prepare produces no detection output.
func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\n",
		"Cargo.lock": "[[package]]\nname = \"app\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)

	test.Assert(t, len(dirs) == 0, "expected no dirs when skipPrepare is true")
	test.Assert(t, len(tools) == 0, "expected no tools when skipPrepare is true")
	test.Assert(t, hash == "", "expected empty hash when skipPrepare is true")
}
