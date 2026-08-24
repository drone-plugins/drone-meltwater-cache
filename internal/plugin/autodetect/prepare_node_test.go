package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestNpmCacheDirsLockfileWithoutConfigUsesHarnessDefault(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)

	test.Equals(t, []string{filepath.Join(dir, "node_modules"), harnessNpmCacheDefault}, dirs)
	_, statErr := os.Stat(filepath.Join(dir, npmrcFileName))
	test.Assert(t, os.IsNotExist(statErr), ".npmrc must not be created")
}

func TestNpmCacheDirsHonorsNpmConfigCacheEnv(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", cacheDir)
	t.Setenv("NPM_CONFIG_CACHE", "")

	dir := t.TempDir()
	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)

	absCache, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), filepath.Clean(absCache)}, dirs)

	_, statErr := os.Stat(filepath.Join(dir, npmrcFileName))
	test.Assert(t, os.IsNotExist(statErr), ".npmrc must not be written when npm_config_cache is set")
}

func TestNpmCacheDirsHonorsUppercaseNpmConfigCacheEnv(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", cacheDir)

	dir := t.TempDir()
	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)

	absCache, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), filepath.Clean(absCache)}, dirs)

	_, statErr := os.Stat(filepath.Join(dir, npmrcFileName))
	test.Assert(t, os.IsNotExist(statErr), ".npmrc must not be written when NPM_CONFIG_CACHE is set")
}

func TestNpmCacheDirsResolvesRelativeNpmConfigCache(t *testing.T) {
	t.Setenv("npm_config_cache", "relative-npm-cache")
	t.Setenv("NPM_CONFIG_CACHE", "")

	dir := t.TempDir()
	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)

	absCache, err := filepath.Abs("relative-npm-cache")
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), filepath.Clean(absCache)}, dirs)
}

func TestNpmCacheDirsEmptyNpmConfigCacheUsesHarnessDefault(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), harnessNpmCacheDefault}, dirs)
}

func TestNpmCacheDirsHonorsExistingNpmrcCache(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	custom := filepath.Join(dir, "from-npmrc")
	original := "registry=https://example.invalid\ncache=" + custom + "\n"
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte(original), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), custom}, dirs)

	got, err := os.ReadFile(filepath.Join(dir, npmrcFileName))
	test.Ok(t, err)
	test.Equals(t, original, string(got))
}

func TestNpmCacheDirsHonorsRelativeCacheInNpmrc(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache=vendor/npm-cache\n"), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, []string{
		filepath.Join(dir, "node_modules"),
		filepath.Join(dir, "vendor", "npm-cache"),
	}, dirs)
}

func TestNpmCacheDirsQuotedAndSpacedCacheInNpmrc(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	custom := filepath.Join(dir, "quoted-cache")
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache = \""+custom+"\"\n"), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, custom, dirs[1])
}

func TestNpmCacheDirsIgnoresCommentedCacheAndCacheMin(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	contents := "; cache=/should-not-use\n# cache=/also-not\ncache-min=0\nregistry=https://example.invalid\n"
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte(contents), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), harnessNpmCacheDefault}, dirs)

	got, err := os.ReadFile(filepath.Join(dir, npmrcFileName))
	test.Ok(t, err)
	test.Equals(t, contents, string(got))
}

func TestNpmCacheDirsLastCacheLineInNpmrcWins(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache="+first+"\ncache="+second+"\n"), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, second, dirs[1])
}

func TestNpmCacheDirsEnvWinsOverNpmrcCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", cacheDir)
	t.Setenv("NPM_CONFIG_CACHE", "")

	dir := t.TempDir()
	npmrcPath := filepath.Join(dir, npmrcFileName)
	original := "cache=" + filepath.Join(dir, "from-npmrc") + "\n"
	test.Ok(t, os.WriteFile(npmrcPath, []byte(original), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)

	absCache, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Clean(absCache), dirs[1])

	got, err := os.ReadFile(npmrcPath)
	test.Ok(t, err)
	test.Equals(t, original, string(got))
}

func TestNpmCacheDirsDoesNotTreatCacheMinAsCache(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache-min=10\n"), 0644))

	dirs, err := npmCacheDirs(dir, true)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), harnessNpmCacheDefault}, dirs)

	got, err := os.ReadFile(filepath.Join(dir, npmrcFileName))
	test.Ok(t, err)
	test.Equals(t, "cache-min=10\n", string(got))
}

func TestNpmCacheDirsPackageJSONOnlyUsesHarnessDefault(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()

	dirs, err := npmCacheDirs(dir, false)
	test.Ok(t, err)

	test.Equals(t, []string{harnessNpmCacheDefault}, dirs)
	_, statErr := os.Stat(filepath.Join(dir, npmrcFileName))
	test.Assert(t, os.IsNotExist(statErr), ".npmrc must not be created")
}

func TestNpmCacheDirsLowercaseEnvWinsOverUppercase(t *testing.T) {
	lower := t.TempDir()
	t.Setenv("npm_config_cache", lower)
	t.Setenv("NPM_CONFIG_CACHE", t.TempDir())

	dirs, err := npmCacheDirs(t.TempDir(), false)
	test.Ok(t, err)

	absLower, err := filepath.Abs(lower)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Clean(absLower)}, dirs)
}

func TestNpmCacheDirsHarnessWorkspaceDoesNotChangeDefault(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Setenv(harnessWorkspaceEnv, t.TempDir())

	dirs, err := npmCacheDirs(t.TempDir(), false)
	test.Ok(t, err)
	test.Equals(t, []string{harnessNpmCacheDefault}, dirs)
}

func TestNpmrcCacheValueMissingFile(t *testing.T) {
	got, err := npmrcCacheValue(filepath.Join(t.TempDir(), "missing.npmrc"))
	test.Ok(t, err)
	test.Equals(t, "", got)
}
