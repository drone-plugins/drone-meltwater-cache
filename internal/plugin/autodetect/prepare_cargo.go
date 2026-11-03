package autodetect

import (
	"os"
	"path/filepath"
)

// cargoPreparer selects downloads, never the Cargo home itself: that home can
// contain credentials, configuration, installed executables and extracted trees.
type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer { return &cargoPreparer{} }

func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoCacheHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "cache"), nil
}

func cargoCacheDirs(dir string) ([]string, error) {
	home, err := cargoCacheHome(dir)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}, nil
}

func cargoCacheHome(dir string) (string, error) {
	if home := os.Getenv("CARGO_HOME"); home != "" {
		// Cargo resolves a relative CARGO_HOME against process cwd, including
		// when the manifest is nested below the workspace root.
		return filepath.Abs(home)
	}
	// Build containers must also receive CARGO_HOME pointing here. A plugin
	// process cannot relocate the cache for a subsequent build container.
	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "cargo"))
}
