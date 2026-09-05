package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestPythonPreparerPoetry(t *testing.T) {
	tempDir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(tempDir, "poetry.lock"), nil, 0644))

	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	pyproject := "[tool.poetry]\nname = \"test\"\n"
	test.Ok(t, os.WriteFile(pyprojectPath, []byte(pyproject), 0644))

	cacheDir, err := newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)

	expectedCacheDir := filepath.Join(tempDir, ".cache", "poetry")
	test.Equals(t, expectedCacheDir, cacheDir)
	content, err := os.ReadFile(filepath.Join(tempDir, "poetry.toml"))
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), `cache-dir = "`+expectedCacheDir+`"`),
		"expected poetry.toml to configure the cache directory")

	unchangedPyproject, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	test.Equals(t, pyproject, string(unchangedPyproject))
}

func TestPythonPreparerPoetryMergesExistingConfig(t *testing.T) {
	tempDir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(tempDir, "poetry.lock"), nil, 0644))
	configPath := filepath.Join(tempDir, "poetry.toml")
	test.Ok(t, os.WriteFile(configPath, []byte("virtualenvs.create = false\ncache-dir = \"/old\"\n"), 0600))

	expectedCacheDir := filepath.Join(tempDir, ".cache", "poetry")
	_, err := newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	_, err = newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)

	content, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), "virtualenvs.create = false"),
		"expected existing Poetry configuration to be preserved")
	test.Equals(t, 1, strings.Count(string(content), "cache-dir"))
	test.Assert(t, strings.Contains(string(content), expectedCacheDir),
		"expected existing cache directory to be replaced")

	info, err := os.Stat(configPath)
	test.Ok(t, err)
	test.Equals(t, os.FileMode(0600), info.Mode().Perm())
}

func TestPythonPreparerPoetryWithoutPyproject(t *testing.T) {
	tempDir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(tempDir, "poetry.lock"), nil, 0644))

	cacheDir, err := newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(tempDir, ".cache", "poetry"), cacheDir)
	test.Assert(t, fileExists(filepath.Join(tempDir, "poetry.toml")),
		"expected poetry.toml to be created")
}

func TestPythonPreparerPipenv(t *testing.T) {
	tempDir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(tempDir, "Pipfile.lock"), nil, 0644))
	pipfilePath := filepath.Join(tempDir, "Pipfile")
	pipfile := "[packages]\nrequests = \"*\"\n"
	test.Ok(t, os.WriteFile(pipfilePath, []byte(pipfile), 0644))
	envPath := filepath.Join(tempDir, ".env")
	test.Ok(t, os.WriteFile(envPath, []byte("EXISTING=value\nexport PIPENV_CACHE_DIR = /old\n"), 0600))

	cacheDir, err := newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	_, err = newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)

	expectedCacheDir := filepath.Join(tempDir, ".cache", "pipenv")
	test.Equals(t, expectedCacheDir, cacheDir)
	content, err := os.ReadFile(envPath)
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), "EXISTING=value"),
		"expected existing .env values to be preserved")
	test.Assert(t, strings.Contains(string(content), "PIPENV_CACHE_DIR="+expectedCacheDir),
		"expected .env to configure Pipenv's cache")
	test.Equals(t, 1, strings.Count(string(content), "PIPENV_CACHE_DIR"))

	unchangedPipfile, err := os.ReadFile(pipfilePath)
	test.Ok(t, err)
	test.Equals(t, pipfile, string(unchangedPipfile))
}

func TestPipPreparer(t *testing.T) {
	tempDir := t.TempDir()

	cacheDir, err := newPipPreparer(".cache/pip").PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(tempDir, ".cache", "pip"), cacheDir)

	absolute := filepath.Join(tempDir, "custom-pip-cache")
	cacheDir, err = newPipPreparer(absolute).PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, absolute, cacheDir)
}

func TestPythonPreparerPriorityPoetryOverPipenv(t *testing.T) {
	tempDir := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(tempDir, "poetry.lock"), nil, 0644))
	test.Ok(t, os.WriteFile(filepath.Join(tempDir, "Pipfile.lock"), nil, 0644))

	cacheDir, err := newPythonPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(tempDir, ".cache", "poetry"), cacheDir)
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.txt")
	test.Assert(t, !fileExists(path), "expected false for nonexistent file")
	test.Ok(t, os.WriteFile(path, nil, 0644))
	test.Assert(t, fileExists(path), "expected true for existing file")
}
