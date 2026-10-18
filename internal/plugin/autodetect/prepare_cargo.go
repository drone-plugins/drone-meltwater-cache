package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

// cargoDefaultHome is the repo-local CARGO_HOME used when the build does not set
// CARGO_HOME. Cargo only honors the environment variable, so later build steps
// must export CARGO_HOME=.cache/cargo for cargo to write to the cached location.
const cargoDefaultHome = ".cache/cargo"

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer {
	return &cargoPreparer{}
}

// cargoHome resolves the effective CARGO_HOME. Cargo's cache location is
// controlled only by the CARGO_HOME environment variable; there is no
// project-local config file that relocates it. When the build already sets
// CARGO_HOME we cache under that path. Otherwise we default to a repo-local
// .cache/cargo, which requires later build steps to export
// CARGO_HOME=.cache/cargo so cargo reads and writes the cached location.
func cargoHome(dir string) (string, error) {
	if envHome := strings.TrimSpace(os.Getenv("CARGO_HOME")); envHome != "" {
		return resolveRepoPath(dir, envHome)
	}
	return resolveRepoPath(dir, cargoDefaultHome)
}

// PrepareRepo returns cargo's registry download cache (the .crate tarballs),
// the most valuable reusable path. No config file is written: the cache
// location is controlled only by CARGO_HOME.
func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "cache"), nil
}

// cargoCacheDirs returns the remaining reusable download caches: the registry
// index and the bare git repositories for git dependencies. registry/src and
// git/checkouts are deliberately excluded because cargo re-extracts them from
// these caches; target/, bin/, and credential files (credentials.toml) are
// never cached. Absent paths (e.g. git/db with no git dependencies) are skipped
// gracefully by the caller.
func cargoCacheDirs(dir string) ([]string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return nil, err
	}
	return []string{
		filepath.Join(home, "registry", "index"),
		filepath.Join(home, "git", "db"),
	}, nil
}
