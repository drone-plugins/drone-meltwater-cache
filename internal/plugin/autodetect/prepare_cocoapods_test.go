package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestCocoapodsPreparerReturnsPods(t *testing.T) {
	for _, osName := range []string{"linux", "darwin"} {
		t.Run(osName, func(t *testing.T) {
			withGOOS(t, osName)

			path, err := newCocoapodsPreparer().PrepareRepo("/some/dir")
			test.Ok(t, err)
			test.Equals(t, filepath.Join("/some/dir", "Pods"), path)
		})
	}
}

func TestCocoapodsCacheDirsLinux(t *testing.T) {
	withGOOS(t, "linux")
	t.Setenv("CP_CACHE_DIR", "")
	t.Setenv("CP_HOME_DIR", "")

	dirs, err := cocoapodsCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
}

func TestCocoapodsCacheDirsDarwin(t *testing.T) {
	withGOOS(t, "darwin")
	t.Setenv("CP_CACHE_DIR", "")
	t.Setenv("CP_HOME_DIR", "")

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	dirs, err := cocoapodsCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{
		filepath.Join(home, "Library", "Caches", "CocoaPods"),
		filepath.Join(home, ".cocoapods", "cache"),
	}, dirs)
}

func TestCocoapodsCacheDirsCPCacheDir(t *testing.T) {
	for _, osName := range []string{"linux", "darwin"} {
		t.Run(osName, func(t *testing.T) {
			withGOOS(t, osName)
			t.Setenv("CP_CACHE_DIR", "/harness/custom/cocoapods-cache")
			t.Setenv("CP_HOME_DIR", "")

			dirs, err := cocoapodsCacheDirs("/some/dir")
			test.Ok(t, err)
			test.Assert(t, len(dirs) > 0, "expected at least one cache dir")
			test.Equals(t, "/harness/custom/cocoapods-cache", dirs[0])
		})
	}
}

func TestCocoapodsCacheDirsCPHomeDir(t *testing.T) {
	for _, osName := range []string{"linux", "darwin"} {
		t.Run(osName, func(t *testing.T) {
			withGOOS(t, osName)
			t.Setenv("CP_CACHE_DIR", "")
			t.Setenv("CP_HOME_DIR", "/harness/custom-home")

			dirs, err := cocoapodsCacheDirs("/some/dir")
			test.Ok(t, err)
			test.Assert(t, len(dirs) > 0, "expected at least one cache dir")
			test.Equals(t, "/harness/custom-home/cache", dirs[0])
		})
	}
}

func TestCocoapodsCacheDirsExpandHomeTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	test.Ok(t, err)

	t.Setenv("CP_CACHE_DIR", "~/custom-cp-cache")
	t.Setenv("CP_HOME_DIR", "")
	withGOOS(t, "linux")

	dirs, err := cocoapodsCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(home, "custom-cp-cache")}, dirs)
}

func TestCocoapodsPreparerDoesNotModifyRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := newCocoapodsPreparer().PrepareRepo(dir)
	test.Ok(t, err)

	after, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(after))
}
