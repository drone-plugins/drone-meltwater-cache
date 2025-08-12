package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestBzlmodPreparerPrepareRepo(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "bzlmod-test")
	test.Ok(t, err)
	defer os.RemoveAll(tempDir)

	// Create a new bzlmodPreparer
	preparer := &bzlmodPreparer{}

	// Test case 1: .bazelrc doesn't exist
	pathToCache, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify the path to cache is correct
	expectedPath := filepath.Join(tempDir, ".bazel")
	test.Equals(t, expectedPath, pathToCache)

	// Verify .bazelrc was created
	bazelrcPath := filepath.Join(tempDir, ".bazelrc")
	_, err = os.Stat(bazelrcPath)
	test.Ok(t, err)

	// Read the content of .bazelrc
	content, err := os.ReadFile(bazelrcPath)
	test.Ok(t, err)

	// Verify the content contains the expected cache paths
	runtimeCache := filepath.Join(expectedPath, "run")
	moduleCache := filepath.Join(expectedPath, "module")
	registryCache := filepath.Join(expectedPath, "registry")

	contentStr := string(content)
	test.Assert(t, strings.Contains(contentStr, runtimeCache), "Content should contain runtime cache path")
	test.Assert(t, strings.Contains(contentStr, moduleCache), "Content should contain module cache path")
	test.Assert(t, strings.Contains(contentStr, registryCache), "Content should contain registry cache path")

	// Test case 2: .bazelrc already exists
	// Add some initial content to .bazelrc
	initialContent := "# Initial bazelrc content\n"
	err = os.WriteFile(bazelrcPath, []byte(initialContent), 0644)
	test.Ok(t, err)

	// Call PrepareRepo again
	pathToCache, err = preparer.PrepareRepo(tempDir)
	test.Ok(t, err)

	// Verify the path to cache is still correct
	test.Equals(t, expectedPath, pathToCache)

	// Read the updated content of .bazelrc
	updatedContent, err := os.ReadFile(bazelrcPath)
	test.Ok(t, err)

	// Verify the content contains both the initial content and the appended cache paths
	updatedContentStr := string(updatedContent)
	test.Assert(t, strings.Contains(updatedContentStr, initialContent), "Content should contain initial content")
	test.Assert(t, strings.Contains(updatedContentStr, runtimeCache), "Content should contain runtime cache path")
	test.Assert(t, strings.Contains(updatedContentStr, moduleCache), "Content should contain module cache path")
	test.Assert(t, strings.Contains(updatedContentStr, registryCache), "Content should contain registry cache path")
}

func TestBzlmodPreparerErrorHandling(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "bzlmod-error-test")
	test.Ok(t, err)
	defer os.RemoveAll(tempDir)

	// Create a new bzlmodPreparer
	preparer := &bzlmodPreparer{}

	// Create a read-only directory to simulate permission errors
	readOnlyDir := filepath.Join(tempDir, "readonly")
	err = os.Mkdir(readOnlyDir, 0500) // read-only directory
	test.Ok(t, err)

	// Test case: Permission error when creating .bazelrc
	_, err = preparer.PrepareRepo(readOnlyDir)
	test.Assert(t, err != nil, "Expected error due to permission issues")
}
