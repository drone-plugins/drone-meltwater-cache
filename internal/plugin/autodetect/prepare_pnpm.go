package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

const pnpmWorkspaceFileName = "pnpm-workspace.yaml"

type pnpmPreparer struct{}

func newPnpmPreparer() *pnpmPreparer { return &pnpmPreparer{} }

func (*pnpmPreparer) PrepareRepo(dir string) (string, error) {
	stores, err := pnpmStoreDirs(dir)
	if err != nil {
		return "", err
	}
	return stores[0], nil
}

// pnpmCacheDirs returns the remaining candidate stores plus node_modules.
// Stores that do not exist at save time are skipped by the rebuilder.
func pnpmCacheDirs(dir string) ([]string, error) {
	stores, err := pnpmStoreDirs(dir)
	if err != nil {
		return nil, err
	}
	return append(stores[1:], filepath.Join(dir, "node_modules")), nil
}

// pnpmStoreDirs lists every store location the installed pnpm may use. pnpm
// <= 10 reads npm_config_store_dir and lets pnpm-workspace.yaml override it;
// pnpm >= 11 reads pnpm_config_store_dir, which overrides the workspace file,
// and ignores .npmrc. The plugin cannot tell which pnpm the build step runs,
// so all configured locations are cached. Nothing is written to the repo.
func pnpmStoreDirs(dir string) ([]string, error) {
	var stores []string

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"pnpm_config_store_dir", "npm_config_store_dir"} {
		if value := os.Getenv(name); strings.TrimSpace(value) != "" {
			store, err := resolveRepoPath(cwd, value)
			if err != nil {
				return nil, err
			}
			stores = appendIfMissing(stores, store)
		}
	}

	configured, err := yamlTopLevelString(filepath.Join(dir, pnpmWorkspaceFileName), "storeDir")
	if err != nil {
		return nil, err
	}
	npmrcStore, err := npmrcValue(filepath.Join(dir, npmrcFileName), "store-dir")
	if err != nil {
		return nil, err
	}
	for _, value := range []string{configured, npmrcStore} {
		if value != "" {
			store, err := resolveRepoPath(dir, value)
			if err != nil {
				return nil, err
			}
			stores = appendIfMissing(stores, store)
		}
	}

	if len(stores) == 0 {
		store, err := resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "pnpm"))
		if err != nil {
			return nil, err
		}
		stores = append(stores, store)
	}

	return stores, nil
}
