package autodetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestBzlmodPreparerPrepareRepo(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Unsetenv("USE_BAZEL_VERSION")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	tempDir, err := os.MkdirTemp("", "bzlmod-test")
	test.Ok(t, err)
	defer os.RemoveAll(tempDir)

	preparer := &bzlmodPreparer{}
	expectedCachePath := filepath.Join(tempDir, bazelCacheDirName)

	// Test case 1: .bazelrc doesn't exist
	pathToCache, err := preparer.PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, expectedCachePath, pathToCache)

	bazelrcPath := filepath.Join(tempDir, ".bazelrc")
	_, err = os.Stat(bazelrcPath)
	test.Ok(t, err)

	content, err := os.ReadFile(bazelrcPath)
	test.Ok(t, err)

	contentStr := string(content)
	expectedRepoCache := "common --repository_cache=" + expectedCachePath
	test.Assert(t, strings.Contains(contentStr, expectedRepoCache),
		"Content should contain common --repository_cache")

	// .bazelignore should be created (cache is inside workspace)
	bazelignorePath := filepath.Join(tempDir, ".bazelignore")
	ignoreContent, err := os.ReadFile(bazelignorePath)
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(ignoreContent), bazelCacheDirName),
		"bazelignore should contain cache dir")

	// Test case 2: .bazelrc already exists
	initialContent := "# Initial bazelrc content\n"
	err = os.WriteFile(bazelrcPath, []byte(initialContent), 0644)
	test.Ok(t, err)

	pathToCache, err = preparer.PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, expectedCachePath, pathToCache)

	updatedContent, err := os.ReadFile(bazelrcPath)
	test.Ok(t, err)
	updatedContentStr := string(updatedContent)

	test.Assert(t, strings.Contains(updatedContentStr, initialContent),
		"Content should contain initial content")
	test.Assert(t, strings.Contains(updatedContentStr, expectedRepoCache),
		"Content should contain common --repository_cache")
}

func TestBzlmodPreparerErrorHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bzlmod-error-test")
	test.Ok(t, err)
	defer os.RemoveAll(tempDir)

	bazelrcPath := filepath.Join(tempDir, ".bazelrc")
	f, err := os.Create(bazelrcPath)
	test.Ok(t, err)
	f.Close()

	originalOsOpenFile := osOpenFile
	defer func() { osOpenFile = originalOsOpenFile }()

	osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return nil, fmt.Errorf("mock permission denied")
	}

	preparer := &bzlmodPreparer{}

	_, err = preparer.PrepareRepo(tempDir)
	test.Assert(t, strings.Contains(err.Error(), "mock permission denied"),
		"Error should contain 'mock permission denied'")
}
