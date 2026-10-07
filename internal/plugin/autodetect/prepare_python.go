package autodetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type poetryPreparer struct{}
type pipenvPreparer struct{}
type pipPreparer struct {
	cacheDir string
	fromEnv  bool
}

func newPoetryPreparer() *poetryPreparer {
	return &poetryPreparer{}
}

func newPipenvPreparer() *pipenvPreparer {
	return &pipenvPreparer{}
}

func newPipPreparer() *pipPreparer {
	if cacheDir := os.Getenv("PIP_CACHE_DIR"); cacheDir != "" {
		return &pipPreparer{cacheDir: cacheDir, fromEnv: true}
	}
	return &pipPreparer{cacheDir: filepath.Join(".cache", "pip")}
}

func (*poetryPreparer) PrepareRepo(dir string) (string, error) {
	return preparePoetry(dir)
}

func (*pipenvPreparer) PrepareRepo(dir string) (string, error) {
	return preparePipenv(dir)
}

// PrepareRepo writes pip.conf so later steps can use PIP_CONFIG_FILE=pip.conf.
// Skipped when PIP_CACHE_DIR is already set, since that env var outranks any config file.
func (p *pipPreparer) PrepareRepo(dir string) (string, error) {
	cacheDir, err := resolveRepoPath(dir, p.cacheDir)
	if err != nil {
		return "", err
	}
	if p.fromEnv {
		return cacheDir, nil
	}
	if err := upsertPipConf(filepath.Join(dir, "pip.conf"), cacheDir); err != nil {
		return "", err
	}
	return cacheDir, nil
}

func resolveRepoPath(dir, cacheDir string) (string, error) {
	if filepath.IsAbs(cacheDir) {
		return filepath.Clean(cacheDir), nil
	}
	path, err := filepath.Abs(filepath.Join(dir, cacheDir))
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

// preparePoetry writes Poetry's project-local cache-dir config, unless
// POETRY_CACHE_DIR is already set (it takes precedence over poetry.toml).
func preparePoetry(dir string) (string, error) {
	if envDir := os.Getenv("POETRY_CACHE_DIR"); envDir != "" {
		return resolveRepoPath(dir, envDir)
	}

	configPath := filepath.Join(dir, "poetry.toml")
	cacheDir := filepath.Join(dir, ".cache", "poetry")
	if err := upsertTOMLString(configPath, "", "cache-dir", cacheDir); err != nil {
		return "", err
	}

	return cacheDir, nil
}

// preparePipenv uses PIPENV_CACHE_DIR when the build already sets it. Otherwise
// it caches a repo-local directory. Pipenv reads that variable at process start,
// so later steps still need PIPENV_CACHE_DIR set to the same path.
func preparePipenv(dir string) (string, error) {
	if envDir := os.Getenv("PIPENV_CACHE_DIR"); envDir != "" {
		return resolveRepoPath(dir, envDir)
	}

	cacheDir := filepath.Join(dir, ".cache", "pipenv")
	if err := upsertEnv(filepath.Join(dir, ".env"), "PIPENV_CACHE_DIR", cacheDir); err != nil {
		return "", err
	}

	return cacheDir, nil
}

func upsertPipConf(path, cacheDir string) error {
	content, mode, err := readOptionalFile(path)
	if err != nil {
		return err
	}
	if iniHasKey(content, "cache-dir") {
		return nil
	}

	block := fmt.Sprintf("[global]\ncache-dir = %s\n", cacheDir)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content != "" {
		block = "\n" + block
	}
	return os.WriteFile(path, []byte(content+block), mode)
}

func iniHasKey(content, key string) bool {
	prefix := key + " "
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) || strings.HasPrefix(trimmed, key+"=") {
			return true
		}
	}
	return false
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
