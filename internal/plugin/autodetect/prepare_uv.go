package autodetect

import (
	"os"
	"path/filepath"
)

type uvPreparer struct{}

func newUvPreparer() *uvPreparer {
	return &uvPreparer{}
}

// PrepareRepo configures uv to use a repo-local cache directory. uv.toml takes
// precedence over pyproject.toml, so update it when present. If neither file
// exists, create uv.toml; this also supports standalone uv lockfiles.
// UV_CACHE_DIR outranks both files, so when the build already sets it we cache
// that path instead of writing a cache-dir uv would ignore.
func (*uvPreparer) PrepareRepo(dir string) (string, error) {
	if envDir := os.Getenv("UV_CACHE_DIR"); envDir != "" {
		return resolveRepoPath(dir, envDir)
	}

	cacheDir := filepath.Join(dir, ".cache", "uv")
	uvConfigPath := filepath.Join(dir, "uv.toml")
	pyprojectPath := filepath.Join(dir, "pyproject.toml")

	if fileExists(uvConfigPath) || !fileExists(pyprojectPath) {
		if err := upsertTOMLString(uvConfigPath, "", "cache-dir", cacheDir); err != nil {
			return "", err
		}
		return cacheDir, nil
	}

	if err := upsertTOMLString(pyprojectPath, "tool.uv", "cache-dir", cacheDir); err != nil {
		return "", err
	}
	return cacheDir, nil
}
