package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

type cocoapodsPreparer struct{}

func newCocoapodsPreparer() *cocoapodsPreparer {
	return &cocoapodsPreparer{}
}

// PrepareRepoMulti returns the directories CocoaPods needs cached: the installed
// pods inside the workspace, and the shared download cache. There is no config
// file to redirect (unlike maven/gradle/yarn), so this preparer only computes
// paths and does not modify the repo.
func (*cocoapodsPreparer) PrepareRepoMulti(dir string) ([]string, error) {
	paths := []string{filepath.Join(dir, "Pods")}

	cacheRoot, err := cocoapodsCacheRoot()
	if err != nil {
		return nil, err
	}

	if cacheRoot != "" {
		paths = append(paths, cacheRoot)
	}

	return paths, nil
}

// cocoapodsCacheRoot mirrors Pod::Config's own resolution order: CP_CACHE_DIR
// wins outright, otherwise CP_HOME_DIR moves the cache to <home>/cache, and
// failing both it is the macOS default.
//
// The cache_root key in ~/.cocoapods/config.yaml is deliberately not read, as
// that would mean taking on a YAML dependency for a rarely-used setting; a
// customer relying on it gets a cache miss rather than a failure, because
// autodetect mounts are registered with WithGracefulDetect.
func cocoapodsCacheRoot() (string, error) {
	if cacheDir := os.Getenv("CP_CACHE_DIR"); cacheDir != "" {
		return expandUserPath(cacheDir)
	}

	if homeDir := os.Getenv("CP_HOME_DIR"); homeDir != "" {
		expanded, err := expandUserPath(homeDir)
		if err != nil {
			return "", err
		}

		return filepath.Join(expanded, "cache"), nil
	}

	// CocoaPods only runs on macOS, so on any other platform there is no shared
	// cache directory worth mounting. Returning the Darwin path regardless would
	// mount a junk ~/Library/... path on Linux and Windows runners.
	if goos() != "darwin" {
		return "", nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, "Library", "Caches", "CocoaPods"), nil
}

// expandUserPath mirrors Ruby's Pathname#expand_path for the forms CocoaPods
// accepts in these env vars: a leading ~ is the user's home, and the result is
// always absolute.
func expandUserPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return filepath.Clean(absPath), nil
}
