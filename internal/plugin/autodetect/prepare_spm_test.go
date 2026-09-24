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

// PrepareRepo must return a workspace-relative path so the injected Save/Restore
// Cache plugin (which runs with HOME=/) can reach it on the shared /harness
// volume. The path must not depend on $HOME on any platform (CI-23961).
func TestSPMPreparerReturnsWorkspaceCheckouts(t *testing.T) {
	for _, osName := range []string{"linux", "darwin"} {
		t.Run(osName, func(t *testing.T) {
			withGOOS(t, osName)

			path, err := newSPMPreparer().PrepareRepo("/some/dir")
			test.Ok(t, err)
			test.Equals(t, filepath.Join("/some/dir", ".build", "checkouts"), path)
			test.Assert(t, filepath.IsAbs(path), "expected absolute path, got %q", path)
		})
	}
}

// On Linux the extra dependency stores are the two remaining workspace-relative
// .build directories only; the $HOME-based global cache is deliberately dropped
// because the cache plugin cannot reach it.
func TestSPMCacheDirsLinux(t *testing.T) {
	withGOOS(t, "linux")

	dirs, err := spmCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{
		filepath.Join("/some/dir", ".build", "repositories"),
		filepath.Join("/some/dir", ".build", "artifacts"),
	}, dirs)
}

// On macOS the shared repository cache is reachable (builds run on the host, not
// in a container), so it is added after the workspace-relative directories.
func TestSPMCacheDirsDarwin(t *testing.T) {
	withGOOS(t, "darwin")

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	dirs, err := spmCacheDirs("/some/dir")
	test.Ok(t, err)
	test.Equals(t, []string{
		filepath.Join("/some/dir", ".build", "repositories"),
		filepath.Join("/some/dir", ".build", "artifacts"),
		filepath.Join(home, "Library", "Caches", "org.swift.swiftpm"),
	}, dirs)
}

func TestSPMCacheDirsXcodeIntegratedOmitsMetadataBuildDir(t *testing.T) {
	withGOOS(t, "darwin")

	home, err := os.UserHomeDir()
	test.Ok(t, err)

	manifest := filepath.Join("App.xcodeproj", "project.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved")
	_, xcode := classifySPMManifest(manifest)
	test.Assert(t, xcode, "expected xcode-integrated manifest")

	dirs, err := spmCacheDirsForManifest(manifest)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(home, "Library", "Caches", "org.swift.swiftpm")}, dirs)
}

func TestSPMCacheDirsXcodeIntegratedLinuxHasNoHomePath(t *testing.T) {
	withGOOS(t, "linux")

	manifest := filepath.Join("App.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved")
	dirs, err := spmCacheDirsForManifest(manifest)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
}

func TestSPMPreparerDoesNotModifyRepo(t *testing.T) {
	withGOOS(t, "linux")

	dir, err := os.MkdirTemp("", "spm-test-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	before, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(before))

	_, err = newSPMPreparer().PrepareRepo(dir)
	test.Ok(t, err)

	_, err = spmCacheDirs(dir)
	test.Ok(t, err)

	after, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(after))
}
