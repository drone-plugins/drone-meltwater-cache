package autodetect

import (
	"os"
	"path/filepath"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer { return &cargoPreparer{} }

// cargoHome selects only the shared home, never reads or rewrites user config.
// Build steps must also set CARGO_HOME; a plugin cannot export it to later steps.
func cargoHome(dir string) (string, error) {
	if home := os.Getenv("CARGO_HOME"); home != "" {
		// Cargo resolves relative CARGO_HOME from cwd, even for nested manifests.
		return resolveRepoPath(".", home)
	}
	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "cargo"))
}

func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "cache"), nil
}

// Select only reusable downloads and index metadata, not Cargo home itself:
// it also holds credentials, config, installed binaries and process state.
func cargoCacheDirs(dir string) ([]string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}, nil
}
