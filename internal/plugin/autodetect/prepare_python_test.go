package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

// TestPythonPreparerPoetry verifies Poetry configuration injection in pyproject.toml
func TestPythonPreparerPoetry(t *testing.T) {
	tempDir := t.TempDir()

	// Create poetry.lock and pyproject.toml
	poetryLock := filepath.Join(tempDir, "poetry.lock")
	f, err := os.Create(poetryLock)
	test.Ok(t, err)
	f.Close()

	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	err = os.WriteFile(pyprojectPath, []byte("[tool.poetry]\nname = \"test\"\n"), 0644)
	test.Ok(t, err)

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify cache directory is repo-local
	expectedCacheDir := filepath.Join(tempDir, ".cache", "poetry")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify pyproject.toml was modified
	content, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	configStr := string(content)
	test.Assert(t, strings.Contains(configStr, "cache-dir"), "expected cache-dir in pyproject.toml")
	test.Assert(t, strings.Contains(configStr, expectedCacheDir), "expected config to contain cache path")
}

// TestPythonPreparerPoetryWithoutPyproject verifies Poetry handles missing pyproject.toml
func TestPythonPreparerPoetryWithoutPyproject(t *testing.T) {
	tempDir := t.TempDir()

	// Create poetry.lock but no pyproject.toml
	poetryLock := filepath.Join(tempDir, "poetry.lock")
	f, err := os.Create(poetryLock)
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	_, err = preparer.PrepareRepo(tempDir)

	// Should error because pyproject.toml doesn't exist
	test.Assert(t, err != nil, "expected error when pyproject.toml missing")
}

// TestPythonPreparerPipenv verifies Pipenv .env.local creation
func TestPythonPreparerPipenv(t *testing.T) {
	tempDir := t.TempDir()

	// Create Pipfile.lock
	pipfileLock := filepath.Join(tempDir, "Pipfile.lock")
	f, err := os.Create(pipfileLock)
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify cache directory is repo-local
	expectedCacheDir := filepath.Join(tempDir, ".cache", "pipenv")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify .env.local was created with PIPENV_CACHE_DIR
	envLocalFile := filepath.Join(tempDir, ".env.local")
	test.Assert(t, fileExists(envLocalFile), "expected .env.local to be created")

	content, err := os.ReadFile(envLocalFile)
	test.Ok(t, err)
	configStr := string(content)
	test.Assert(t, strings.Contains(configStr, "PIPENV_CACHE_DIR"), "expected PIPENV_CACHE_DIR in .env.local")
	test.Assert(t, strings.Contains(configStr, expectedCacheDir), "expected cache path in .env.local")
}

// TestPythonPreparerRequirementsTxt verifies pip configuration with setup.cfg
func TestPythonPreparerRequirementsTxt(t *testing.T) {
	tempDir := t.TempDir()

	// Create requirements.txt
	reqFile := filepath.Join(tempDir, "requirements.txt")
	f, err := os.Create(reqFile)
	test.Ok(t, err)
	_, err = f.WriteString("requests==2.28.0\n")
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify cache directory is repo-local
	expectedCacheDir := filepath.Join(tempDir, ".cache", "pip")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify setup.cfg was created with cache configuration
	setupCfg := filepath.Join(tempDir, "setup.cfg")
	test.Assert(t, fileExists(setupCfg), "expected setup.cfg to be created")

	// Verify setup.cfg contains cache-dir setting
	content, err := os.ReadFile(setupCfg)
	test.Ok(t, err)
	configStr := string(content)
	test.Assert(t, strings.Contains(configStr, "[global]"), "expected [global] section in setup.cfg")
	test.Assert(t, strings.Contains(configStr, expectedCacheDir), "expected cache path in setup.cfg")
}

// TestPythonPreparerExistingSetupCfg verifies pip appends to existing setup.cfg
func TestPythonPreparerExistingSetupCfg(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing setup.cfg
	setupCfg := filepath.Join(tempDir, "setup.cfg")
	f, err := os.Create(setupCfg)
	test.Ok(t, err)
	_, err = f.WriteString("[metadata]\nname = test\n")
	test.Ok(t, err)
	f.Close()

	// Create requirements.txt
	reqFile := filepath.Join(tempDir, "requirements.txt")
	f, err = os.Create(reqFile)
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify cache directory is repo-local
	expectedCacheDir := filepath.Join(tempDir, ".cache", "pip")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify setup.cfg was updated and contains both old and new config
	content, err := os.ReadFile(setupCfg)
	test.Ok(t, err)
	configStr := string(content)
	test.Assert(t, strings.Contains(configStr, "[metadata]"), "expected original [metadata] section to be preserved")
	test.Assert(t, strings.Contains(configStr, "[global]"), "expected new [global] section to be added")
	test.Assert(t, strings.Contains(configStr, expectedCacheDir), "expected cache path in setup.cfg")
}

// Priority test: poetry.lock takes precedence over requirements.txt
func TestPythonPreparerPriorityPoetryOverPip(t *testing.T) {
	tempDir := t.TempDir()

	// Create both poetry.lock and requirements.txt
	poetryLock := filepath.Join(tempDir, "poetry.lock")
	f, err := os.Create(poetryLock)
	test.Ok(t, err)
	f.Close()

	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	err = os.WriteFile(pyprojectPath, []byte("[tool.poetry]\nname = \"test\"\n"), 0644)
	test.Ok(t, err)

	reqFile := filepath.Join(tempDir, "requirements.txt")
	f, err = os.Create(reqFile)
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Should prefer poetry
	expectedCacheDir := filepath.Join(tempDir, ".cache", "poetry")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify poetry config was created
	content, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), "cache-dir"), "expected cache-dir in pyproject.toml")
}

// Priority test: Pipfile.lock takes precedence over requirements.txt
func TestPythonPreparerPriorityPipenvOverPip(t *testing.T) {
	tempDir := t.TempDir()

	// Create both Pipfile.lock and requirements.txt
	pipfileLock := filepath.Join(tempDir, "Pipfile.lock")
	f, err := os.Create(pipfileLock)
	test.Ok(t, err)
	f.Close()

	reqFile := filepath.Join(tempDir, "requirements.txt")
	f, err = os.Create(reqFile)
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Should prefer pipenv
	expectedCacheDir := filepath.Join(tempDir, ".cache", "pipenv")
	test.Equals(t, cacheDir, expectedCacheDir)
}

// Priority test: poetry.lock > Pipfile.lock > requirements.txt
func TestPythonPreparerPriorityAll(t *testing.T) {
	tempDir := t.TempDir()

	// Create all three
	poetryLock := filepath.Join(tempDir, "poetry.lock")
	f, err := os.Create(poetryLock)
	test.Ok(t, err)
	f.Close()

	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	err = os.WriteFile(pyprojectPath, []byte("[tool.poetry]\nname = \"test\"\n"), 0644)
	test.Ok(t, err)

	pipfileLock := filepath.Join(tempDir, "Pipfile.lock")
	f, err = os.Create(pipfileLock)
	test.Ok(t, err)
	f.Close()

	reqFile := filepath.Join(tempDir, "requirements.txt")
	f, err = os.Create(reqFile)
	test.Ok(t, err)
	f.Close()

	preparer := newPythonPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Should prefer poetry (highest priority)
	expectedCacheDir := filepath.Join(tempDir, ".cache", "poetry")
	test.Equals(t, cacheDir, expectedCacheDir)
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()

	// File doesn't exist
	test.Assert(t, !fileExists(filepath.Join(tempDir, "nonexistent.txt")), "expected false for nonexistent file")

	// Create a file
	filePath := filepath.Join(tempDir, "test.txt")
	f, err := os.Create(filePath)
	test.Ok(t, err)
	f.Close()

	// File exists
	test.Assert(t, fileExists(filePath), "expected true for existing file")
}
