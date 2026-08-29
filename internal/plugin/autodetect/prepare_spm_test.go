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

func TestSPMPreparerDarwin(t *testing.T) {
	withGOOS(t, "darwin")

	origXDG := os.Getenv("XDG_CACHE_HOME")
	os.Unsetenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", origXDG)

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	path, err := newSPMPreparer().PrepareRepo("/some/dir")
	test.Ok(t, err)
	test.Equals(t, filepath.Join(home, "Library", "Caches", "org.swift.swiftpm"), path)
}

func TestSPMPreparerLinuxNoXDG(t *testing.T) {
	withGOOS(t, "linux")

	origXDG := os.Getenv("XDG_CACHE_HOME")
	os.Unsetenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", origXDG)

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	path, err := newSPMPreparer().PrepareRepo("/some/dir")
	test.Ok(t, err)
	test.Equals(t, filepath.Join(home, ".cache", "org.swift.swiftpm"), path)
}

func TestSPMPreparerLinuxWithXDG(t *testing.T) {
	withGOOS(t, "linux")

	origXDG := os.Getenv("XDG_CACHE_HOME")
	os.Setenv("XDG_CACHE_HOME", "/custom/cache")
	defer os.Setenv("XDG_CACHE_HOME", origXDG)

	path, err := newSPMPreparer().PrepareRepo("/some/dir")
	test.Ok(t, err)
	test.Equals(t, filepath.Join("/custom/cache", "org.swift.swiftpm"), path)
}

// The XDG spec requires absolute paths and says relative values must be treated
// as invalid; honouring one would hand the cache layer a relative mount point.
func TestSPMPreparerLinuxIgnoresRelativeXDG(t *testing.T) {
	withGOOS(t, "linux")

	t.Setenv("XDG_CACHE_HOME", "relative/cache")

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	path, err := newSPMPreparer().PrepareRepo("/some/dir")
	test.Ok(t, err)
	test.Equals(t, filepath.Join(home, ".cache", "org.swift.swiftpm"), path)
	test.Assert(t, filepath.IsAbs(path), "expected absolute path, got %q", path)
}

func TestSPMPreparerDoesNotModifyRepo(t *testing.T) {
	withGOOS(t, "darwin")

	dir, err := os.MkdirTemp("", "spm-test-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	before, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(before))

	_, err = newSPMPreparer().PrepareRepo(dir)
	test.Ok(t, err)

	after, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(after))
}
