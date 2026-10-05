package autodetect

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func isolateComposerEnv(t *testing.T) {
	t.Helper()
	t.Setenv("COMPOSER_VENDOR_DIR", "")
	t.Setenv("COMPOSER_CACHE_DIR", "")
	t.Setenv("COMPOSER_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HARNESS_WORKSPACE", "")
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
}

func TestComposerPreparer_Default(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "vendor")
	test.Equals(t, vendorDir, expected)
}

func TestComposerPreparer_EnvVarOverride_Relative(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	t.Setenv("COMPOSER_VENDOR_DIR", "custom_vendor")

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "custom_vendor")
	test.Equals(t, vendorDir, expected)
}

func TestComposerPreparer_EnvVarOverride_Absolute(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()
	absVendor := filepath.Join(t.TempDir(), "abs_vendor")

	t.Setenv("COMPOSER_VENDOR_DIR", absVendor)

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	test.Equals(t, vendorDir, absVendor)
}

func TestComposerPreparer_ComposerJSON_VendorDir(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, []byte(`{
		"name": "acme/project",
		"config": {
			"vendor-dir": "deps"
		}
	}`), 0644)
	test.Ok(t, err)

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "deps")
	test.Equals(t, vendorDir, expected)
}

func TestComposerPreparer_EnvVar_Outranks_ComposerJSON(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, []byte(`{
		"name": "acme/project",
		"config": {
			"vendor-dir": "json_vendor"
		}
	}`), 0644)
	test.Ok(t, err)

	t.Setenv("COMPOSER_VENDOR_DIR", "env_vendor")

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "env_vendor")
	test.Equals(t, vendorDir, expected)
}

func TestComposerPreparer_MalformedJSON(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, []byte(`{ invalid json syntax`), 0644)
	test.Ok(t, err)

	preparer := newComposerPreparer()
	_, err = preparer.PrepareRepo(dir)
	test.Assert(t, err != nil, "expected error on malformed composer.json")
}

func TestComposerPreparer_EmptyJSON(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, []byte(""), 0644)
	test.Ok(t, err)

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "vendor")
	test.Equals(t, vendorDir, expected)
}

func TestComposerPreparer_FallbackPreparer(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	preparer := newComposerFallbackPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "vendor")
	test.Equals(t, vendorDir, expected)
}

func TestComposerCacheDirs_Default_OutsideWorkspace(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	// Default user home cache is outside temporary workspace directory
	extraDirs, err := composerCacheDirs(dir)
	test.Ok(t, err)
	test.Assert(t, len(extraDirs) == 0, "expected no extra cache dirs outside workspace")
}

func TestComposerCacheDirs_EnvVar_Relative(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	t.Setenv("COMPOSER_CACHE_DIR", ".cache/composer")

	extraDirs, err := composerCacheDirs(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, ".cache", "composer")
	test.Equals(t, extraDirs, []string{expected})
}

func TestComposerCacheDirs_EnvVar_Absolute(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()
	absCache := filepath.Join(t.TempDir(), "global-composer-cache")

	t.Setenv("COMPOSER_CACHE_DIR", absCache)

	extraDirs, err := composerCacheDirs(dir)
	test.Ok(t, err)

	test.Equals(t, extraDirs, []string{absCache})
}

func TestComposerCacheDirs_ComposerJSON_CacheDir(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, []byte(`{
		"name": "acme/project",
		"config": {
			"cache-dir": ".composer-cache"
		}
	}`), 0644)
	test.Ok(t, err)

	extraDirs, err := composerCacheDirs(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, ".composer-cache")
	test.Equals(t, extraDirs, []string{expected})
}

func TestComposerCacheDirs_EnvVar_Outranks_ComposerJSON(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, []byte(`{
		"name": "acme/project",
		"config": {
			"cache-dir": "json_cache"
		}
	}`), 0644)
	test.Ok(t, err)

	t.Setenv("COMPOSER_CACHE_DIR", "env_cache")

	extraDirs, err := composerCacheDirs(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "env_cache")
	test.Equals(t, extraDirs, []string{expected})
}

func TestComposerCacheDirs_InsideWorkspace_XDG(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	t.Setenv("HARNESS_WORKSPACE", dir)
	xdgDir := filepath.Join(dir, ".xdg-cache")
	t.Setenv("XDG_CACHE_HOME", xdgDir)

	extraDirs, err := composerCacheDirs(dir)
	test.Ok(t, err)

	expected := filepath.Join(xdgDir, "composer")
	test.Equals(t, extraDirs, []string{expected})
}

func TestComposer_ZeroMutation_PreservesFile(t *testing.T) {
	isolateComposerEnv(t)
	dir := t.TempDir()

	initialContent := []byte(`{
		"name": "acme/project",
		"description": "testing zero mutation",
		"require": {
			"monolog/monolog": "^2.0"
		},
		"config": {
			"vendor-dir": "custom_vendor"
		}
	}`)
	composerJSON := filepath.Join(dir, "composer.json")
	err := os.WriteFile(composerJSON, initialContent, 0644)
	test.Ok(t, err)

	preparer := newComposerPreparer()
	vendorDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, vendorDir, filepath.Join(dir, "custom_vendor"))

	_, err = composerCacheDirs(dir)
	test.Ok(t, err)

	afterContent, err := os.ReadFile(composerJSON)
	test.Ok(t, err)
	test.Assert(t, bytes.Equal(initialContent, afterContent), "composer.json must remain strictly untouched")
}
