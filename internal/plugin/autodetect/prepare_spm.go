package autodetect

import (
	"os"
	"path/filepath"
	"runtime"
)

// goos is a package-level indirection over runtime.GOOS so tests can exercise
// both platform branches without depending on the test host's actual OS.
var goos = func() string { return runtime.GOOS }

const spmBuildDir = ".build"

type spmPreparer struct{}

func newSPMPreparer() *spmPreparer {
	return &spmPreparer{}
}

// PrepareRepo returns the directory that holds Swift Package Manager's
// checked-out dependency sources (.build/checkouts). SPM has no config file to
// redirect, so the repo is not modified.
//
// The Linux ~/.cache/org.swift.swiftpm cache is deliberately not used: Harness
// runs the injected cache plugin in its own container with HOME=/, so a
// $HOME-relative path is never the one `swift build` populated. .build lives in
// the shared workspace, so every container sees it.
func (*spmPreparer) PrepareRepo(dir string) (string, error) {
	return filepath.Join(dir, spmBuildDir, "checkouts"), nil
}

// spmCacheDirs returns the package-local bare git clones (.build/repositories)
// and, on macOS, the shared ~/Library/Caches/org.swift.swiftpm cache. macOS
// builds run on the host, so HOME is the same for the build and the cache step.
func spmCacheDirs(dir string) ([]string, error) {
	dirs := []string{filepath.Join(dir, spmBuildDir, "repositories")}

	if goos() == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		dirs = append(dirs, filepath.Join(home, "Library", "Caches", "org.swift.swiftpm"))
	}

	return dirs, nil
}
