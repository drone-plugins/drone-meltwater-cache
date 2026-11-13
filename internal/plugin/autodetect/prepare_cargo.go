package autodetect

import (
	"os"
	"path/filepath"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer { return &cargoPreparer{} }

// cargoHome follows Cargo's cwd-relative CARGO_HOME semantics. The default
// matches Harness's shared-workspace injection; build steps must also set it.
// Never archive the whole home: it contains config, credentials and binaries.
func cargoHome(dir string) (string, error) {
	if home := os.Getenv("CARGO_HOME"); home != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, home)
	}
	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "cargo"))
}

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
