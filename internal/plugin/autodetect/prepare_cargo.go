package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

// Cargo stores everything under CARGO_HOME (default ~/.cargo). Only the
// download/index caches are worth persisting; the extracted sources and
// checkouts are re-created from them, and bin/config/credentials must never
// be cached.
const (
	cargoRegistryCacheRel = "registry/cache"
	cargoRegistryIndexRel = "registry/index"
	cargoGitDBRel         = "git/db"
)

type cargoPreparer struct{}

func newCargoPreparer() *cargoPreparer {
	return &cargoPreparer{}
}

// PrepareRepo returns Cargo's registry download-cache directory.
//
// Cargo has no project-local configuration file that can relocate CARGO_HOME:
// the variable must be set before the process starts, so a config file written
// into the repository would be ignored. The plugin therefore follows CARGO_HOME
// when the build already exports it, and otherwise targets a repo-local
// .cache/cargo. For the repo-local default to be consumed, later build steps
// must export CARGO_HOME=.cache/cargo (relative to the project directory).
//
// Only registry/cache, registry/index and git/db are cached (see
// cargoAdditionalCacheDirs). registry/src and git/checkouts are deliberately
// excluded because Cargo re-extracts them from the download caches, and the
// bin/, config.toml and credentials.toml entries are never cached.
func (*cargoPreparer) PrepareRepo(dir string) (string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return "", err
	}

	return filepath.Join(home, filepath.FromSlash(cargoRegistryCacheRel)), nil
}

// cargoAdditionalCacheDirs returns the registry index and git database caches
// that complement the primary registry download cache. These paths may be
// absent before the first build; graceful detection skips absent paths on save.
func cargoAdditionalCacheDirs(dir string) ([]string, error) {
	home, err := cargoHome(dir)
	if err != nil {
		return nil, err
	}

	return []string{
		filepath.Join(home, filepath.FromSlash(cargoRegistryIndexRel)),
		filepath.Join(home, filepath.FromSlash(cargoGitDBRel)),
	}, nil
}

// cargoHome resolves the directory Cargo uses as CARGO_HOME.
// Precedence:
//  1. CARGO_HOME environment variable (follow-only; relative values resolve
//     against the project directory).
//  2. Repo-local .cache/cargo default.
func cargoHome(dir string) (string, error) {
	if envHome := strings.TrimSpace(os.Getenv("CARGO_HOME")); envHome != "" {
		return resolveRepoPath(dir, envHome)
	}

	return resolveRepoPath(dir, filepath.Join(".cache", "cargo"))
}
