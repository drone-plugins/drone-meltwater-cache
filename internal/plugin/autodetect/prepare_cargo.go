package autodetect

import (
	"os"
	"path/filepath"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer { return &cargoPreparer{} }

func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "cache"), nil
}

// cargoHome selects the same home in restore and save without modifying Cargo
// configuration. Build steps must also set CARGO_HOME to this shared location.
func cargoHome(dir string) (string, error) {
	if home := os.Getenv("CARGO_HOME"); home != "" {
		// Cargo resolves relative CARGO_HOME against the invocation cwd, not the
		// directory of a nested Cargo.lock. Do not trim valid path whitespace.
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, home)
	}
	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "cargo"))
}

// Cache only re-usable downloads and the index needed for offline resolution,
// never the whole Cargo home (which also holds configuration and credentials).
func cargoCacheDirs(dir string) ([]string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}, nil
}
