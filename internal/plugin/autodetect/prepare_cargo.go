package autodetect

import (
	"os"
	"path/filepath"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer { return &cargoPreparer{} }

// PrepareRepo selects only downloaded crate archives, never the entire Cargo
// home (which can contain credentials and installed executables).
func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "cache"), nil
}

func cargoCacheDirs(dir string) ([]string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}, nil
}

// Cargo resolves relative CARGO_HOME against its invocation directory, not the
// directory containing Cargo.lock. Restore, install and save must share cwd/env.
// No Cargo CLI is needed in the plugin image and no native config is rewritten.
func cargoHome(dir string) (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if home := os.Getenv("CARGO_HOME"); home != "" {
		return resolveRepoPath(root, home)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		candidate, err := resolveRepoPath(root, filepath.Join(home, ".cargo"))
		if err != nil {
			return "", err
		}
		if pathWithin(candidate, root) {
			return candidate, nil
		}
	}
	// Cargo has no project-local setting for its home. This is a suggested shared
	// workspace path, not a relocation: later steps MUST set CARGO_HOME accordingly.
	return filepath.Join(root, ".cache", "cargo"), nil
}
