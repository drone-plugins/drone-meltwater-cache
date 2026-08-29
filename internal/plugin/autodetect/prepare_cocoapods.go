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

// PrepareRepo returns the workspace Pods/ directory. CocoaPods has no config
// file to redirect, so this only computes the path. The shared download cache
// is a second directory, reported by cocoapodsCacheDirs and mounted through
// buildToolInfo.additionalCacheDirs, the same way npm and Python report extras.
func (*cocoapodsPreparer) PrepareRepo(dir string) (string, error) {
	return filepath.Join(dir, "Pods"), nil
}

// cocoapodsCacheDirs returns the shared CocoaPods download cache, or nothing
// when that cache does not apply (non-macOS, and no CP_CACHE_DIR / CP_HOME_DIR).
func cocoapodsCacheDirs(string) ([]string, error) {
	cacheRoot, err := cocoapodsCacheRoot()
	if err != nil || cacheRoot == "" {
		return nil, err
	}

	return []string{cacheRoot}, nil
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
