package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestNodePreparerDefaultCachesNodeModulesAndTarballDir(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)

	test.Equals(t, []string{
		filepath.Join(dir, "node_modules"),
		filepath.Join(dir, npmCacheDirName),
	}, dirs)

	got, err := newNodePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "node_modules"), got)

	data, err := os.ReadFile(filepath.Join(dir, npmrcFileName))
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(data), "cache="+filepath.Join(dir, npmCacheDirName)),
		"expected .npmrc to contain workspace tarball cache, got %q", data)
}

func TestNodePreparerHonorsNpmConfigCacheEnv(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", cacheDir)
	t.Setenv("NPM_CONFIG_CACHE", "")

	dir := t.TempDir()
	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)

	absCache, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), filepath.Clean(absCache)}, dirs)

	_, statErr := os.Stat(filepath.Join(dir, npmrcFileName))
	test.Assert(t, os.IsNotExist(statErr), ".npmrc must not be written when npm_config_cache is set")
}

func TestNodePreparerHonorsUppercaseNpmConfigCacheEnv(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", cacheDir)

	dir := t.TempDir()
	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)

	absCache, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), filepath.Clean(absCache)}, dirs)
}

func TestNodePreparerResolvesRelativeNpmConfigCache(t *testing.T) {
	t.Setenv("npm_config_cache", "relative-npm-cache")
	t.Setenv("NPM_CONFIG_CACHE", "")

	dir := t.TempDir()
	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)

	absCache, err := filepath.Abs("relative-npm-cache")
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), filepath.Clean(absCache)}, dirs)
}

func TestNodePreparerEmptyNpmConfigCacheFallsBackToDotNpm(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, npmCacheDirName), dirs[1])
}

func TestNodePreparerHonorsExistingNpmrcCache(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	custom := filepath.Join(dir, "from-npmrc")
	original := "registry=https://example.invalid\ncache=" + custom + "\n"
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte(original), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "node_modules"), custom}, dirs)

	got, err := os.ReadFile(filepath.Join(dir, npmrcFileName))
	test.Ok(t, err)
	test.Equals(t, original, string(got))
}

func TestNodePreparerHonorsRelativeCacheInNpmrc(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache=vendor/npm-cache\n"), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, []string{
		filepath.Join(dir, "node_modules"),
		filepath.Join(dir, "vendor", "npm-cache"),
	}, dirs)
}

func TestNodePreparerQuotedAndSpacedCacheInNpmrc(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	custom := filepath.Join(dir, "quoted-cache")
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache = \""+custom+"\"\n"), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, custom, dirs[1])
}

func TestNodePreparerIgnoresCommentedCacheAndCacheMin(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	contents := "; cache=/should-not-use\n# cache=/also-not\ncache-min=0\nregistry=https://example.invalid\n"
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte(contents), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, npmCacheDirName), dirs[1])

	got, err := os.ReadFile(filepath.Join(dir, npmrcFileName))
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(got), "registry=https://example.invalid"),
		"existing .npmrc keys must be preserved")
	test.Assert(t, strings.Contains(string(got), "cache="+filepath.Join(dir, npmCacheDirName)),
		"expected cache= to be appended, got %q", got)
}

func TestNodePreparerLastCacheLineInNpmrcWins(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache="+first+"\ncache="+second+"\n"), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, second, dirs[1])
}

func TestNodePreparerEnvWinsOverNpmrcCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", cacheDir)
	t.Setenv("NPM_CONFIG_CACHE", "")

	dir := t.TempDir()
	npmrcPath := filepath.Join(dir, npmrcFileName)
	original := "cache=" + filepath.Join(dir, "from-npmrc") + "\n"
	test.Ok(t, os.WriteFile(npmrcPath, []byte(original), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)

	absCache, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Clean(absCache), dirs[1])

	got, err := os.ReadFile(npmrcPath)
	test.Ok(t, err)
	test.Equals(t, original, string(got))
}

func TestNodePreparerDoesNotTreatCacheMinAsCache(t *testing.T) {
	clearNpmCacheEnv(t)
	dir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(dir, npmrcFileName), []byte("cache-min=10\n"), 0644))

	dirs, err := newNodePreparer().PrepareRepoDirs(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, npmCacheDirName), dirs[1])
}

func TestNpmrcCacheValueMissingFile(t *testing.T) {
	got, err := npmrcCacheValue(filepath.Join(t.TempDir(), "missing.npmrc"))
	test.Ok(t, err)
	test.Equals(t, "", got)
}
