package autodetect

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// goos is a package-level indirection over runtime.GOOS so tests can exercise
// both platform branches without depending on the test host's actual OS.
var goos = func() string { return runtime.GOOS }

const spmBuildDir = ".build"

type spmPreparer struct{}

func newSPMPreparer() *spmPreparer {
	return &spmPreparer{}
}

// PrepareRepo returns the workspace-relative directory that holds Swift Package
// Manager's checked-out dependency sources (.build/checkouts).
//
// SPM has no config file to redirect its cache location (only the --cache-path
// CLI flag, which this plugin cannot inject into the customer's build command),
// so this preparer does not modify the repo. It deliberately does NOT return the
// global ~/.cache/org.swift.swiftpm cache on Linux: Harness Cache Intelligence
// runs the injected Save/Restore Cache plugin in its own container with HOME=/,
// so a $HOME-relative path is never the one `swift build` actually populated and
// the cache silently saves nothing (CI-23961). .build lives under the package
// root, which is inside the shared /harness workspace, so it is reachable no
// matter what HOME each container sees. The remaining dependency stores are
// reported by spmCacheDirs, mirroring how cocoapods reports its extra cache dir.
func (*spmPreparer) PrepareRepo(dir string) (string, error) {
	return filepath.Join(dir, spmBuildDir, "checkouts"), nil
}

// spmCacheDirs returns SPM's remaining dependency stores, wired through
// buildToolInfo.additionalCacheDirs the same way cocoapods reports its shared
// download cache. All of these are dependency inputs, not compiled build
// products, so they stay valid for the manifest-hash cache key:
//
//   - .build/repositories: package-local bare git clones of dependencies.
//   - .build/artifacts:    downloaded binaryTarget artifacts (e.g. xcframeworks).
//
// On macOS the shared ~/Library/Caches/org.swift.swiftpm cache is added too.
// Harness runs macOS builds directly on the host rather than in a container, so
// HOME is consistent between the build and the injected cache plugin, and both
// `swift build` and Xcode's integrated SwiftPM share that repository cache.
func spmCacheDirs(dir string) ([]string, error) {
	dirs := []string{
		filepath.Join(dir, spmBuildDir, "repositories"),
		filepath.Join(dir, spmBuildDir, "artifacts"),
	}

	if goos() == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		dirs = append(dirs, filepath.Join(home, "Library", "Caches", "org.swift.swiftpm"))
	}

	return dirs, nil
}

// xcodePackageResolvedSuffix is the path tail Xcode writes for a package
// resolved through an xcodeproj or xcworkspace. Those files do not own a
// .build directory; Xcode stores the shared dependency cache under the user
// Library directory.
const xcodePackageResolvedSuffix = "xcshareddata/swiftpm/Package.resolved"

// classifySPMManifest reports whether resolvedPath belongs to an Xcode project.
// Standalone packages return the directory that contains Package.swift.
func classifySPMManifest(resolvedPath string) (projectDir string, xcodeIntegrated bool) {
	if strings.HasSuffix(filepath.ToSlash(resolvedPath), xcodePackageResolvedSuffix) {
		return "", true
	}

	return filepath.Dir(resolvedPath), false
}

// spmCacheDirsForManifest returns the dependency directories for one manifest.
// Standalone packages use the workspace .build inputs. Xcode-integrated
// resolution uses the macOS shared SwiftPM cache and does not invent a .build
// tree under xcshareddata/swiftpm.
func spmCacheDirsForManifest(resolvedPath string) ([]string, error) {
	projectDir, xcodeIntegrated := classifySPMManifest(resolvedPath)
	if xcodeIntegrated {
		if goos() != "darwin" {
			return nil, nil
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		return []string{filepath.Join(home, "Library", "Caches", "org.swift.swiftpm")}, nil
	}

	var err error
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}

	checkouts, err := newSPMPreparer().PrepareRepo(projectDir)
	if err != nil {
		return nil, err
	}

	dirs := []string{checkouts}
	extra, err := spmCacheDirs(projectDir)
	if err != nil {
		return nil, err
	}

	return append(dirs, extra...), nil
}
