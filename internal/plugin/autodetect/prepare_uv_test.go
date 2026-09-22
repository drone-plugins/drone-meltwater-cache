package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

// TestUvPreparerBasic verifies uv configuration injection in pyproject.toml
func TestUvPreparerBasic(t *testing.T) {
	tempDir := t.TempDir()

	// Create uv.lock and pyproject.toml
	uvLock := filepath.Join(tempDir, "uv.lock")
	f, err := os.Create(uvLock)
	test.Ok(t, err)
	f.Close()

	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	err = os.WriteFile(pyprojectPath, []byte("[project]\nname = \"test\"\n"), 0644)
	test.Ok(t, err)

	preparer := newUvPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify cache directory is repo-local
	expectedCacheDir := filepath.Join(tempDir, ".cache", "uv")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify pyproject.toml was modified
	content, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	configStr := string(content)
	test.Assert(t, strings.Contains(configStr, "[tool.uv]"), "expected [tool.uv] section in pyproject.toml")
	test.Assert(t, strings.Contains(configStr, "cache-dir"), "expected cache-dir in pyproject.toml")
	test.Assert(t, strings.Contains(configStr, expectedCacheDir), "expected cache path in pyproject.toml")
}

// TestUvPreparerExistingToolUv verifies uv appends to existing [tool.uv] section
func TestUvPreparerExistingToolUv(t *testing.T) {
	tempDir := t.TempDir()

	// Create uv.lock and pyproject.toml with existing [tool.uv]
	uvLock := filepath.Join(tempDir, "uv.lock")
	f, err := os.Create(uvLock)
	test.Ok(t, err)
	f.Close()

	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	err = os.WriteFile(pyprojectPath, []byte("[project]\nname = \"test\"\n\n[tool.uv]\nindexes = []\n"), 0644)
	test.Ok(t, err)

	preparer := newUvPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify cache directory is repo-local
	expectedCacheDir := filepath.Join(tempDir, ".cache", "uv")
	test.Equals(t, cacheDir, expectedCacheDir)

	// Verify pyproject.toml was updated
	content, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	configStr := string(content)
	test.Assert(t, strings.Contains(configStr, "[tool.uv]"), "expected [tool.uv] section to be preserved")
	test.Assert(t, strings.Contains(configStr, "indexes"), "expected original [tool.uv] config to be preserved")
	test.Assert(t, strings.Contains(configStr, "cache-dir"), "expected cache-dir to be added")
	test.Assert(t, strings.Contains(configStr, expectedCacheDir), "expected cache path in pyproject.toml")
}

// TestUvPreparerWithoutPyproject verifies standalone uv projects use uv.toml.
func TestUvPreparerWithoutPyproject(t *testing.T) {
	tempDir := t.TempDir()

	uvLock := filepath.Join(tempDir, "uv.lock")
	f, err := os.Create(uvLock)
	test.Ok(t, err)
	f.Close()

	preparer := newUvPreparer()
	cacheDir, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	expectedCacheDir := filepath.Join(tempDir, ".cache", "uv")
	test.Equals(t, expectedCacheDir, cacheDir)
	content, err := os.ReadFile(filepath.Join(tempDir, "uv.toml"))
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), `cache-dir = "`+expectedCacheDir+`"`),
		"expected uv.toml to configure the cache")
}

func TestUvPreparerUsesUvTomlWhenPresent(t *testing.T) {
	tempDir := t.TempDir()
	uvConfigPath := filepath.Join(tempDir, "uv.toml")
	test.Ok(t, os.WriteFile(uvConfigPath, []byte("offline = true\ncache-dir = \"/old\"\n"), 0644))
	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	pyproject := "[project]\nname = \"test\"\n"
	test.Ok(t, os.WriteFile(pyprojectPath, []byte(pyproject), 0644))

	preparer := newUvPreparer()
	_, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)
	_, err = preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	expectedCacheDir := filepath.Join(tempDir, ".cache", "uv")
	content, err := os.ReadFile(uvConfigPath)
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(content), "offline = true"),
		"expected existing uv.toml settings to be preserved")
	test.Assert(t, strings.Contains(string(content), expectedCacheDir),
		"expected uv.toml cache-dir to be updated")
	test.Equals(t, 1, strings.Count(string(content), "cache-dir"))

	unchangedPyproject, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	test.Equals(t, pyproject, string(unchangedPyproject))
}

func TestUvPreparerScopesCacheDirToToolUv(t *testing.T) {
	tempDir := t.TempDir()
	pyprojectPath := filepath.Join(tempDir, "pyproject.toml")
	pyproject := "[tool.other]\ncache-dir = \"/other\"\n\n[tool.uv]\noffline = true\n"
	test.Ok(t, os.WriteFile(pyprojectPath, []byte(pyproject), 0644))

	_, err := newUvPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)

	content, err := os.ReadFile(pyprojectPath)
	test.Ok(t, err)
	expectedCacheDir := filepath.Join(tempDir, ".cache", "uv")
	test.Assert(t, strings.Contains(string(content), `cache-dir = "`+expectedCacheDir+`"`),
		"expected cache-dir in [tool.uv]")
	test.Assert(t, strings.Contains(string(content), `cache-dir = "/other"`),
		"expected other table to remain unchanged")
}
