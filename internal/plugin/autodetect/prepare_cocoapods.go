package autodetect

import (
	"os"
	"path/filepath"
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

// cocoapodsCacheDirs returns the shared ~/Library/Caches/CocoaPods download
// cache on macOS, and nothing elsewhere: that path does not exist on other
// platforms.
func cocoapodsCacheDirs(string) ([]string, error) {
	if goos() != "darwin" {
		return nil, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return []string{filepath.Join(home, "Library", "Caches", "CocoaPods")}, nil
}
