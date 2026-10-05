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
// file to redirect, so the repo is not modified.
func (*cocoapodsPreparer) PrepareRepo(dir string) (string, error) {
	return filepath.Join(dir, "Pods"), nil
}

// cocoapodsCacheDirs returns the CocoaPods download caches.
// If CP_CACHE_DIR or CP_HOME_DIR are set, those directories are included on any platform.
// On macOS, the standard download cache (~/Library/Caches/CocoaPods) and the pod cache
// (~/.cocoapods/cache) are also included by default.
func cocoapodsCacheDirs(string) ([]string, error) {
	var dirs []string

	if cacheDir := strings.TrimSpace(os.Getenv("CP_CACHE_DIR")); cacheDir != "" {
		expanded, err := expandUserPath(cacheDir)
		if err != nil {
			return nil, err
		}
		dirs = appendIfMissing(dirs, expanded)
	}

	if homeDir := strings.TrimSpace(os.Getenv("CP_HOME_DIR")); homeDir != "" {
		expanded, err := expandUserPath(homeDir)
		if err != nil {
			return nil, err
		}
		dirs = appendIfMissing(dirs, filepath.Join(expanded, "cache"))
	}

	if goos() == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		dirs = appendIfMissing(dirs, filepath.Join(home, "Library", "Caches", "CocoaPods"))
		dirs = appendIfMissing(dirs, filepath.Join(home, ".cocoapods", "cache"))
	}

	return dirs, nil
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
