package autodetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type pythonPreparer struct{}
type pipPreparer struct {
	cacheDir string
}

func newPythonPreparer() *pythonPreparer {
	return &pythonPreparer{}
}

// PrepareRepo injects cache configuration for Poetry or Pipenv.
// Priority: poetry.lock > Pipfile.lock.
func (*pythonPreparer) PrepareRepo(dir string) (string, error) {
	if fileExists(filepath.Join(dir, "poetry.lock")) {
		return preparePoetry(dir)
	}

	if fileExists(filepath.Join(dir, "Pipfile.lock")) {
		return preparePipenv(dir)
	}

	return "", fmt.Errorf("unsupported Python project in %s", dir)
}

func newPipPreparer(cacheDir string) *pipPreparer {
	return &pipPreparer{cacheDir: cacheDir}
}

// PrepareRepo returns the PIP_CACHE_DIR used by the build. pip has no
// repository-local configuration file that it discovers automatically, so
// auto-detection enables pip only when this environment variable is provided.
func (p *pipPreparer) PrepareRepo(dir string) (string, error) {
	if filepath.IsAbs(p.cacheDir) {
		return filepath.Clean(p.cacheDir), nil
	}

	path, err := filepath.Abs(filepath.Join(dir, p.cacheDir))
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

// preparePoetry writes Poetry's project-local application configuration.
// Poetry deliberately keeps poetry.toml separate from package metadata in
// pyproject.toml.
func preparePoetry(dir string) (string, error) {
	cacheDir := filepath.Join(dir, ".cache", "poetry")
	configPath := filepath.Join(dir, "poetry.toml")
	if err := upsertTOMLString(configPath, "", "cache-dir", cacheDir); err != nil {
		return "", err
	}

	return cacheDir, nil
}

// preparePipenv adds PIPENV_CACHE_DIR to the .env file that Pipenv
// automatically loads. It deliberately leaves Pipfile untouched so its lock
// hash remains valid.
func preparePipenv(dir string) (string, error) {
	cacheDir := filepath.Join(dir, ".cache", "pipenv")
	if err := upsertEnv(filepath.Join(dir, ".env"), "PIPENV_CACHE_DIR", cacheDir); err != nil {
		return "", err
	}

	return cacheDir, nil
}

func upsertEnv(path, key, value string) error {
	content, mode, err := readOptionalFile(path)
	if err != nil {
		return err
	}

	var lines []string
	if content != "" {
		lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	replacement := key + "=" + value
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			lines[i] = replacement
			found = true
		}
	}
	if !found {
		lines = append(lines, replacement)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), mode)
}

func readOptionalFile(path string) (string, os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0644, nil
		}
		return "", 0, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	return string(content), info.Mode().Perm(), nil
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
