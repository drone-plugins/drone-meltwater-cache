package autodetect

import (
	"os"
	"path/filepath"
)

type conanPreparer struct{}

func newConanPreparer() *conanPreparer { return &conanPreparer{} }

// conanHome follows cwd-relative CONAN_HOME semantics, even for nested
// detection signals. The fallback matches Harness's workspace injection.
// Never archive the whole CONAN_HOME: it contains credentials.json, global.conf,
// remotes.json and user profiles.
func conanHome(dir string) (string, error) {
	if configured := os.Getenv("CONAN_HOME"); configured != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, configured)
	}
	return resolveRepoPath(workspaceRoot(dir), ".conan2")
}

// PrepareRepo returns only Conan 2's package cache subdirectory (<CONAN_HOME>/p),
// which holds built packages and cache.sqlite3 while excluding credentials and config.
func (*conanPreparer) PrepareRepo(dir string) (string, error) {
	home, err := conanHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "p"), nil
}
