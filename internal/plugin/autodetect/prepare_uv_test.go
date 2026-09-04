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

// TestUvPreparerWithoutPyproject verifies uv handles missing pyproject.toml
func TestUvPreparerWithoutPyproject(t *testing.T) {
	tempDir := t.TempDir()

	// Create uv.lock but no pyproject.toml
	uvLock := filepath.Join(tempDir, "uv.lock")
	f, err := os.Create(uvLock)
	test.Ok(t, err)
	f.Close()

	preparer := newUvPreparer()
	_, err = preparer.PrepareRepo(tempDir)

	// Should error because pyproject.toml doesn't exist
	test.Assert(t, err != nil, "expected error when pyproject.toml missing")
}
