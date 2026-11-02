package autodetect

import (
	"os"
	"path/filepath"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer { return &cargoPreparer{} }

// Cargo home contains configuration, credentials and installed binaries too.
// Select only dependency download/index leaves; never archive the entire home.
func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "index"), nil
}

func cargoCacheDirs(dir string) ([]string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, "registry", "cache"), filepath.Join(home, "git", "db")}, nil
}

func cargoHome(dir string) (string, error) {
	// Relative CARGO_HOME is process-cwd relative, even for nested manifests.
	if home := os.Getenv("CARGO_HOME"); home != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, home)
	}
	// Build steps must also set CARGO_HOME to this shared workspace location.
	return filepath.Join(workspaceRoot(dir), ".cache", "cargo"), nil
}
