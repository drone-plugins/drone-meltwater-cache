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
	return []string{
		filepath.Join(root, ".cache", "cargo", "registry", "cache"),
		filepath.Join(root, ".cache", "cargo", "registry", "index"),
		filepath.Join(root, ".cache", "cargo", "git", "db"),
	}
}

// Scenario 1: Binary crate with committed lockfile and manifest.
func TestScenario_Cargo_WithLockfile(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n[dependencies]\nserde = \"1\"\n",
		"Cargo.lock": "# v1\n[[package]]\nname = \"serde\"\nversion = \"1.0.0\"\n",
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
		"Cargo.toml": "[package]\nname = \"lib\"\nversion = \"0.1.0\"\n[dependencies]\nlog = \"0.4\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, cargoExpectedDirs(tmpDir), dirs)
}

// Scenario 3: Lockfile wins over manifest fallback (single detection, no dup).
func TestScenario_Cargo_LockfilePrecedence(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n[[package]]\nname = \"serde\"\nversion = \"1.0.0\"\n",
	})

	_, tools, lockHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"cargo"}, tools)

	// Hash must come from Cargo.lock, not Cargo.toml.
	manifestOnly := calcMd5(t, "Cargo.lock")
	test.Equals(t, manifestOnly, lockHash)
}

// Scenario 4: Nested crate one level down.
func TestScenario_Cargo_NestedProject(t *testing.T) {
	isolateCargoEnv(t)
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"service/Cargo.toml": "[package]\nname = \"service\"\nversion = \"0.1.0\"\n",
		"service/Cargo.lock": "# v1\n[[package]]\nname = \"tokio\"\nversion = \"1.0.0\"\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, cargoExpectedDirs(filepath.Join(tmpDir, "service")), dirs)
}

// Scenario 5: CARGO_HOME env override is followed.
func TestScenario_Cargo_EnvVar_CargoHome(t *testing.T) {
	isolateCargoEnv(t)
	t.Setenv("CARGO_HOME", "vendor/cargo")
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	expected := []string{
		filepath.Join(tmpDir, "vendor", "cargo", "registry", "cache"),
		filepath.Join(tmpDir, "vendor", "cargo", "registry", "index"),
		filepath.Join(tmpDir, "vendor", "cargo", "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

// Scenario 6: Hybrid repo (Rust + Node) caches both managers.
func TestScenario_Cargo_Hybrid_WithNode(t *testing.T) {
	isolateCargoEnv(t)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	tmpDir := setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml":        "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock":        "# v1\n[[package]]\nname = \"serde\"\nversion = \"1.0.0\"\n",
		"package.json":      `{"name": "frontend", "dependencies": {"vue": "^3.0"}}`,
		"package-lock.json": `{"name": "frontend", "lockfileVersion": 2}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"node", "cargo"}, tools)
	test.Assert(t, hash != "", "expected non-empty composite hash")

	expected := append(
		[]string{filepath.Join(tmpDir, "node_modules")},
		cargoExpectedDirs(tmpDir)...,
	)
	test.Equals(t, expected, dirs)
}

// Scenario 7: Key invalidation when Cargo.lock changes.
func TestScenario_Cargo_KeyInvalidation(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n[[package]]\nname = \"serde\"\nversion = \"1.0.0\"\n",
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	err = os.WriteFile("Cargo.lock", []byte("# v1\n[[package]]\nname = \"serde\"\nversion = \"2.0.0\"\n"), 0644)
	test.Ok(t, err)

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, hash1 != hash2, "expected key invalidation when Cargo.lock changes")
}

// Scenario 8: Hash is a 32-char lowercase hex MD5 digest.
func TestScenario_Cargo_HashShape(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n",
	})

	_, _, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, 32, len(hash))
	for _, c := range hash {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		test.Assert(t, isHex, "expected lowercase hex digest")
	}
}

// Scenario 9: Idempotency and zero mutation of the working tree.
func TestScenario_Cargo_Idempotency(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n",
	})

	dirs1, tools1, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	dirs2, tools2, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, dirs1, dirs2)
	test.Equals(t, tools1, tools2)
	test.Equals(t, hash1, hash2)

	entries, err := os.ReadDir(".")
	test.Ok(t, err)
	test.Equals(t, 2, len(entries)) // only Cargo.toml and Cargo.lock
}

// Scenario 10: skipPrepare produces no dirs/tools/hash.
func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	isolateCargoEnv(t)
	setupCargoScenarioDir(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": "# v1\n",
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)

	test.Assert(t, len(dirs) == 0, "expected no dirs when skipPrepare is true")
	test.Assert(t, len(tools) == 0, "expected no tools when skipPrepare is true")
	test.Assert(t, hash == "", "expected empty hash when skipPrepare is true")
}

func calcMd5(t *testing.T, path string) string {
	t.Helper()
	hash, _, err := hashIfFileExist(path)
	test.Ok(t, err)
	return hash
}
