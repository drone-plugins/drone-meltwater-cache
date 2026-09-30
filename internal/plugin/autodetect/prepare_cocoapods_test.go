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

	dirs, err := cocoapodsCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
}

func TestCocoapodsCacheDirsDarwin(t *testing.T) {
	withGOOS(t, "darwin")

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	dirs, err := cocoapodsCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(home, "Library", "Caches", "CocoaPods")}, dirs)
}

func TestCocoapodsPreparerDoesNotModifyRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := newCocoapodsPreparer().PrepareRepo(dir)
	test.Ok(t, err)

	after, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(after))
}
