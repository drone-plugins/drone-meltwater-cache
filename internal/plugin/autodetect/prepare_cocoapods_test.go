package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

// isolateCocoapodsEnv gives the test a private HOME and clears the env vars
// Pod::Config consults, so results never depend on the developer's machine.
func isolateCocoapodsEnv(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, key := range []string{"CP_CACHE_DIR", "CP_HOME_DIR"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}

	return home
}

// cocoapodsPaths is what detection mounts: Pods/ from PrepareRepo, then the
// shared cache from additionalCacheDirs.
func cocoapodsPaths(t *testing.T, dir string) []string {
	t.Helper()

	pods, err := newCocoapodsPreparer().PrepareRepo(dir)
	test.Ok(t, err)

	extra, err := cocoapodsCacheDirs(dir)
	test.Ok(t, err)

	return append([]string{pods}, extra...)
}

func TestCocoapodsPreparerDarwinDefaults(t *testing.T) {
	home := isolateCocoapodsEnv(t)
	withGOOS(t, "darwin")

	dir := t.TempDir()

	test.Equals(t, []string{
		filepath.Join(dir, "Pods"),
		filepath.Join(home, "Library", "Caches", "CocoaPods"),
	}, cocoapodsPaths(t, dir))
}

// CocoaPods is macOS-only; returning the Darwin path on other platforms would
// mount a junk ~/Library/... directory on Linux and Windows runners.
func TestCocoapodsPreparerNonDarwinOmitsSharedCache(t *testing.T) {
	isolateCocoapodsEnv(t)
	withGOOS(t, "linux")

	dir := t.TempDir()

	test.Equals(t, []string{filepath.Join(dir, "Pods")}, cocoapodsPaths(t, dir))
}

func TestCocoapodsPreparerRespectsCPCacheDir(t *testing.T) {
	isolateCocoapodsEnv(t)
	withGOOS(t, "darwin")
	t.Setenv("CP_CACHE_DIR", "/custom/pod/cache")

	dir := t.TempDir()

	test.Equals(t, []string{filepath.Join(dir, "Pods"), "/custom/pod/cache"}, cocoapodsPaths(t, dir))
}

// CP_HOME_DIR relocates the cache to <home>/cache, per Pod::Config.
func TestCocoapodsPreparerRespectsCPHomeDir(t *testing.T) {
	isolateCocoapodsEnv(t)
	withGOOS(t, "darwin")
	t.Setenv("CP_HOME_DIR", "/opt/cocoapods-home")

	dir := t.TempDir()

	test.Equals(t, []string{filepath.Join(dir, "Pods"), "/opt/cocoapods-home/cache"}, cocoapodsPaths(t, dir))
}

func TestCocoapodsPreparerCPCacheDirBeatsCPHomeDir(t *testing.T) {
	isolateCocoapodsEnv(t)
	withGOOS(t, "darwin")
	t.Setenv("CP_HOME_DIR", "/opt/cocoapods-home")
	t.Setenv("CP_CACHE_DIR", "/custom/pod/cache")

	dir := t.TempDir()

	test.Equals(t, []string{filepath.Join(dir, "Pods"), "/custom/pod/cache"}, cocoapodsPaths(t, dir))
}

func TestCocoapodsPreparerExpandsTildeAndRelativePaths(t *testing.T) {
	home := isolateCocoapodsEnv(t)
	withGOOS(t, "darwin")
	t.Setenv("CP_CACHE_DIR", "~/pods-cache")

	dir := t.TempDir()

	paths := cocoapodsPaths(t, dir)
	test.Equals(t, []string{filepath.Join(dir, "Pods"), filepath.Join(home, "pods-cache")}, paths)
	test.Assert(t, filepath.IsAbs(paths[1]), "expected absolute path, got %q", paths[1])
}

func TestCocoapodsPreparerDoesNotModifyRepo(t *testing.T) {
	isolateCocoapodsEnv(t)
	withGOOS(t, "darwin")

	dir := t.TempDir()

	_, err := newCocoapodsPreparer().PrepareRepo(dir)
	test.Ok(t, err)
	_, err = cocoapodsCacheDirs(dir)
	test.Ok(t, err)

	after, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(after))
}
