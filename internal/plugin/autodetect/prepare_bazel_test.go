package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestIsBazel9OrNewer_FromFile(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Unsetenv("USE_BAZEL_VERSION")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"no file", "", false},
		{"7.5.0", "7.5.0", false},
		{"8.0.0", "8.0.0", false},
		{"9.0.0", "9.0.0", true},
		{"9.1.0", "9.1.0", true},
		{"10.0.0", "10.0.0", true},
		{"with trailing newline", "9.0.0\n", true},
		{"invalid version", "abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "bazel-ver-file-*")
			test.Ok(t, err)
			defer os.RemoveAll(dir)

			if tt.content != "" {
				test.Ok(t, os.WriteFile(filepath.Join(dir, ".bazelversion"), []byte(tt.content), 0644))
			}
			got := isBazel9OrNewer(dir)
			test.Assert(t, got == tt.expected,
				"isBazel9OrNewer(.bazelversion=%q) = %v, want %v", tt.content, got, tt.expected)
		})
	}
}

func TestIsBazel9OrNewer_FromEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		expected bool
	}{
		{"env 7.5.0", "7.5.0", false},
		{"env 8.0.0", "8.0.0", false},
		{"env 9.0.0", "9.0.0", true},
		{"env 10.0.0", "10.0.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origEnv := os.Getenv("USE_BAZEL_VERSION")
			os.Setenv("USE_BAZEL_VERSION", tt.envVal)
			defer os.Setenv("USE_BAZEL_VERSION", origEnv)

			dir, err := os.MkdirTemp("", "bazel-ver-env-*")
			test.Ok(t, err)
			defer os.RemoveAll(dir)

			got := isBazel9OrNewer(dir)
			test.Assert(t, got == tt.expected,
				"isBazel9OrNewer(USE_BAZEL_VERSION=%q) = %v, want %v", tt.envVal, got, tt.expected)
		})
	}
}

func TestIsBazel9OrNewer_EnvTakesPrecedence(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Setenv("USE_BAZEL_VERSION", "7.5.0")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	dir, err := os.MkdirTemp("", "bazel-ver-precedence-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	// .bazelversion says 9.0.0, but env says 7.5.0 — env wins
	test.Ok(t, os.WriteFile(filepath.Join(dir, ".bazelversion"), []byte("9.0.0"), 0644))
	got := isBazel9OrNewer(dir)
	test.Assert(t, !got, "USE_BAZEL_VERSION=7.5.0 should take precedence over .bazelversion=9.0.0")
}

func TestBazelPreparerNoRepoContentsCacheForBazel7(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Unsetenv("USE_BAZEL_VERSION")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	dir, err := os.MkdirTemp("", "bazel-prep-7-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	test.Ok(t, os.WriteFile(filepath.Join(dir, ".bazelversion"), []byte("7.5.0"), 0644))

	preparer := &bazelPreparer{}
	pathToCache, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expectedCachePath := filepath.Join(dir, bazelCacheDirName)
	test.Equals(t, expectedCachePath, pathToCache)

	content, err := os.ReadFile(filepath.Join(dir, ".bazelrc"))
	test.Ok(t, err)
	contentStr := string(content)

	test.Assert(t, strings.Contains(contentStr, "common --repository_cache="+expectedCachePath),
		"should contain repository_cache")
	test.Assert(t, !strings.Contains(contentStr, "repo_contents_cache"),
		"Bazel 7: should NOT add repo_contents_cache")

	// .bazelignore should be created (cache is inside workspace)
	ignoreContent, err := os.ReadFile(filepath.Join(dir, ".bazelignore"))
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(ignoreContent), bazelCacheDirName),
		".bazelignore should contain cache dir")
}

func TestBazelPreparerAddsRepoContentsCacheForBazel9(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Unsetenv("USE_BAZEL_VERSION")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	dir, err := os.MkdirTemp("", "bazel-prep-9-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	test.Ok(t, os.WriteFile(filepath.Join(dir, ".bazelversion"), []byte("9.0.0"), 0644))

	preparer := &bazelPreparer{}
	_, err = preparer.PrepareRepo(dir)
	test.Ok(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".bazelrc"))
	test.Ok(t, err)
	contentStr := string(content)

	test.Assert(t, strings.Contains(contentStr, "common --repository_cache="),
		"should contain repository_cache")
	test.Assert(t, strings.Contains(contentStr, "common --repo_contents_cache="),
		"Bazel 9: should add repo_contents_cache")
}

func TestBazelPreparerRespectsEnvVar(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Setenv("USE_BAZEL_VERSION", "7.5.0")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	dir, err := os.MkdirTemp("", "bazel-prep-env-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	// No .bazelversion file, but env var is set to 7.5.0
	preparer := &bazelPreparer{}
	_, err = preparer.PrepareRepo(dir)
	test.Ok(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".bazelrc"))
	test.Ok(t, err)
	contentStr := string(content)

	test.Assert(t, strings.Contains(contentStr, "common --repository_cache="),
		"should contain repository_cache")
	test.Assert(t, !strings.Contains(contentStr, "repo_contents_cache"),
		"USE_BAZEL_VERSION=7.5.0: should NOT add repo_contents_cache")
}

func TestBazelPreparerAppendsToBazelrc(t *testing.T) {
	origEnv := os.Getenv("USE_BAZEL_VERSION")
	os.Unsetenv("USE_BAZEL_VERSION")
	defer os.Setenv("USE_BAZEL_VERSION", origEnv)

	dir, err := os.MkdirTemp("", "bazel-append-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	initialContent := "build --remote_cache=http://example.com/cache\n"
	test.Ok(t, os.WriteFile(filepath.Join(dir, ".bazelrc"), []byte(initialContent), 0644))

	preparer := &bazelPreparer{}
	_, err = preparer.PrepareRepo(dir)
	test.Ok(t, err)

	content, err := os.ReadFile(filepath.Join(dir, ".bazelrc"))
	test.Ok(t, err)
	contentStr := string(content)

	test.Assert(t, strings.Contains(contentStr, initialContent),
		"should preserve existing .bazelrc content")
	test.Assert(t, strings.Contains(contentStr, "common --repository_cache="),
		"should append repository_cache")
}
