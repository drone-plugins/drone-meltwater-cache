package autodetect

import (
	"os"
	"path/filepath"
	"runtime"
)

// goos is a package-level indirection over runtime.GOOS so tests can exercise
// both platform branches without depending on the test host's actual OS.
var goos = func() string { return runtime.GOOS }

type spmPreparer struct{}

func newSPMPreparer() *spmPreparer {
	return &spmPreparer{}
}

// PrepareRepo returns the platform-specific Swift Package Manager cache directory.
// SPM has no config file to redirect its cache location (only the --cache-path
// CLI flag, which this plugin cannot inject into the customer's build command),
// so this preparer computes the default path and does not modify the repo.
func (*spmPreparer) PrepareRepo(dir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if goos() == "darwin" {
		return filepath.Join(home, "Library", "Caches", "org.swift.swiftpm"), nil
	}

	// The XDG base directory spec requires these paths to be absolute and says a
	// relative value must be treated as invalid and ignored. IsAbs also rejects
	// the unset case, so this covers both.
	if xdgCache := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(xdgCache) {
		return filepath.Join(xdgCache, "org.swift.swiftpm"), nil
	}

	return filepath.Join(home, ".cache", "org.swift.swiftpm"), nil
}
