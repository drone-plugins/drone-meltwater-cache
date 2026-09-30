package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func withGOOS(t *testing.T, value string) {
	t.Helper()
	orig := goos
	goos = func() string { return value }
	t.Cleanup(func() { goos = orig })
}

// The injected cache plugin runs with HOME=/, so the checkouts path must live in
// the workspace on every platform, never under $HOME.
func TestSPMPreparerReturnsWorkspaceCheckouts(t *testing.T) {
	for _, osName := range []string{"linux", "darwin"} {
		t.Run(osName, func(t *testing.T) {
			withGOOS(t, osName)

			path, err := newSPMPreparer().PrepareRepo("/some/dir")
			test.Ok(t, err)
			test.Equals(t, filepath.Join("/some/dir", ".build", "checkouts"), path)
		})
	}
}

func TestSPMCacheDirsLinux(t *testing.T) {
	withGOOS(t, "linux")

	dirs, err := spmCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join("/some/dir", ".build", "repositories")}, dirs)
}

func TestSPMCacheDirsDarwin(t *testing.T) {
	withGOOS(t, "darwin")

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	dirs, err := spmCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{
		filepath.Join("/some/dir", ".build", "repositories"),
		filepath.Join(home, "Library", "Caches", "org.swift.swiftpm"),
	}, dirs)
}

func TestSPMPreparerDoesNotModifyRepo(t *testing.T) {
	withGOOS(t, "linux")

	dir := t.TempDir()

	_, err := newSPMPreparer().PrepareRepo(dir)
	test.Ok(t, err)

	_, err = spmCacheDirs(dir)
	test.Ok(t, err)

	after, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(after))
}
