package autodetect

import (
	"os"
	"path/filepath"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer {
	return &cargoPreparer{}
}

// PrepareRepo returns Cargo's primary download cache: registry/cache, the
// directory of downloaded ".crate" tarballs. Cargo resolves its cache location
// from CARGO_HOME only; there is no project-local config file it auto-discovers.
// When CARGO_HOME is set the plugin follows it, otherwise it uses a repo-local
// default (<dir>/.cache/cargo). No tracked files are written, so later build
// steps must export CARGO_HOME to the same path for the relocation to take
// effect (see README). registry/src and git/checkouts are re-extracted from
// these caches and are deliberately not cached.
func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := effectiveCargoHome(dir)
	if err != nil {
		return "", err
	}

	return filepath.Join(home, "registry", "cache"), nil
}

// cargoCacheDirs resolves the extra Cargo caches worth keeping: the registry
// index and the bare git-dependency clones. Both live under CARGO_HOME and are
// portable, checksum-verified download caches.
func cargoCacheDirs(dir string) ([]string, error) {
	home, err := effectiveCargoHome(dir)
	if err != nil {
		return nil, err
	}

	return []string{
		filepath.Join(home, "registry", "index"),
		filepath.Join(home, "git", "db"),
	}, nil
}

// effectiveCargoHome returns the CARGO_HOME the plugin caches under. An explicit
// CARGO_HOME wins; otherwise the repo-local default <dir>/.cache/cargo is used so
// the cache sits on the shared workspace volume.
func effectiveCargoHome(dir string) (string, error) {
	if envHome := os.Getenv("CARGO_HOME"); envHome != "" {
		return resolveRepoPath(dir, envHome)
	}

	return resolveRepoPath(dir, filepath.Join(".cache", "cargo"))
}
