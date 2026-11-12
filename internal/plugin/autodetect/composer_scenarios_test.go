package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func setupComposerScenarioDir(t *testing.T, files map[string]string) string {
	t.Helper()
	origDir, err := os.Getwd()
	test.Ok(t, err)

	tmpDir, err := os.MkdirTemp("", "drone-cache-composer-scenario-*")
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

// Scenario 1: Standard app with lockfile and manifest
func TestScenario_Composer_WithLockfile(t *testing.T) {
	isolateComposerEnv(t)
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app", "require": {"monolog/monolog": "^2.0"}}`,
		"composer.lock": `{"_readme": ["lockfile content v1"], "packages": []}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, []string{filepath.Join(tmpDir, "vendor")}, dirs)
}

// Scenario 2: Library with manifest only (no composer.lock)
func TestScenario_Composer_FallbackManifestOnly(t *testing.T) {
	isolateComposerEnv(t)
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/library", "require": {"psr/log": "^1.0"}}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, []string{filepath.Join(tmpDir, "vendor")}, dirs)
}

// Scenario 3: Nested PHP project
func TestScenario_Composer_NestedProject(t *testing.T) {
	isolateComposerEnv(t)
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"backend/composer.json": `{"name": "backend/app"}`,
		"backend/composer.lock": `{"packages": [{"name": "symfony/console"}]}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, []string{filepath.Join(tmpDir, "backend", "vendor")}, dirs)
}

// Scenario 4: Custom COMPOSER_VENDOR_DIR env var
func TestScenario_Composer_EnvVar_VendorDir(t *testing.T) {
	isolateComposerEnv(t)
	t.Setenv("COMPOSER_VENDOR_DIR", "custom_vendor")
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app"}`,
		"composer.lock": `{"packages": []}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, []string{filepath.Join(tmpDir, "custom_vendor")}, dirs)
}

// Scenario 5: Configured vendor-dir in composer.json
func TestScenario_Composer_JSONConfig_VendorDir(t *testing.T) {
	isolateComposerEnv(t)
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app", "config": {"vendor-dir": "project_deps"}}`,
		"composer.lock": `{"packages": []}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	test.Equals(t, []string{filepath.Join(tmpDir, "project_deps")}, dirs)
}

// Scenario 6: Dual layer with COMPOSER_CACHE_DIR
func TestScenario_Composer_DualLayer_WithCacheDir(t *testing.T) {
	isolateComposerEnv(t)
	t.Setenv("COMPOSER_CACHE_DIR", ".cache/composer")
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app"}`,
		"composer.lock": `{"packages": []}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty hash")
	expected := []string{
		filepath.Join(tmpDir, "vendor"),
		filepath.Join(tmpDir, ".cache", "composer"),
	}
	test.Equals(t, expected, dirs)
}

// Scenario 7: Full-stack hybrid repo (PHP + Node)
func TestScenario_Composer_Hybrid_WithNode(t *testing.T) {
	isolateComposerEnv(t)
	tmpDir := setupComposerScenarioDir(t, map[string]string{
		"composer.json":     `{"name": "test/app"}`,
		"composer.lock":     `{"packages": [{"name": "laravel/framework"}]}`,
		"package.json":      `{"name": "frontend", "dependencies": {"vue": "^3.0"}}`,
		"package-lock.json": `{"name": "frontend", "lockfileVersion": 2}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"node", "composer"}, tools)
	test.Assert(t, hash != "", "expected non-empty composite hash")

	expectedDirs := []string{
		filepath.Join(tmpDir, "node_modules"),
		filepath.Join(tmpDir, "vendor"),
	}
	test.Equals(t, expectedDirs, dirs)
}

// Scenario 8: Key invalidation on lockfile change
func TestScenario_Composer_KeyInvalidation(t *testing.T) {
	isolateComposerEnv(t)
	setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app"}`,
		"composer.lock": `{"packages": [{"name": "foo", "version": "1.0.0"}]}`,
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// Update lockfile
	err = os.WriteFile("composer.lock", []byte(`{"packages": [{"name": "foo", "version": "2.0.0"}]}`), 0644)
	test.Ok(t, err)

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, hash1 != hash2, "expected key invalidation when composer.lock changes")
}

// Scenario 9: Idempotency (multiple runs produce identical state and no mutations)
func TestScenario_Composer_Idempotency(t *testing.T) {
	isolateComposerEnv(t)
	setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app", "config": {"vendor-dir": "deps"}}`,
		"composer.lock": `{"packages": []}`,
	})

	initialJSON, err := os.ReadFile("composer.json")
	test.Ok(t, err)

	dirs1, tools1, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	dirs2, tools2, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, dirs1, dirs2)
	test.Equals(t, tools1, tools2)
	test.Equals(t, hash1, hash2)

	afterJSON, err := os.ReadFile("composer.json")
	test.Ok(t, err)
	test.Equals(t, string(initialJSON), string(afterJSON))
}

// Scenario 10: Skip prepare
func TestScenario_Composer_SkipPrepare(t *testing.T) {
	isolateComposerEnv(t)
	setupComposerScenarioDir(t, map[string]string{
		"composer.json": `{"name": "test/app"}`,
		"composer.lock": `{"packages": []}`,
	})

	dirs, tools, hash, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)

	test.Assert(t, len(dirs) == 0, "expected no dirs when skipPrepare is true")
	test.Assert(t, len(tools) == 0, "expected no tools when skipPrepare is true")
	test.Assert(t, hash == "", "expected empty hash when skipPrepare is true")
}
