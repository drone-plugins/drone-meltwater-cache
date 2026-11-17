package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

type bunPreparer struct{}

func newBunPreparer() *bunPreparer { return &bunPreparer{} }

// PrepareRepo resolves Bun's install cache the way Bun does:
// BUN_INSTALL_CACHE_DIR (cwd-relative), then [install.cache] dir in
// bunfig.toml (project-relative), then $BUN_INSTALL/install/cache. The
// default matches Harness's shared-workspace injection. Nothing is written.
func (*bunPreparer) PrepareRepo(dir string) (string, error) {
	if value := os.Getenv("BUN_INSTALL_CACHE_DIR"); strings.TrimSpace(value) != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, value)
	}

	configured, err := tomlStringValue(filepath.Join(dir, "bunfig.toml"), "install.cache", "dir")
	if err != nil {
		return "", err
	}
	if configured != "" {
		return resolveRepoPath(dir, configured)
	}

	if home := os.Getenv("BUN_INSTALL"); strings.TrimSpace(home) != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, filepath.Join(home, "install", "cache"))
	}

	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "bun"))
}
