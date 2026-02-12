package autodetect

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// bazelCacheDirName is the cache directory for Bazel
// Using .bazel-cache relative to project directory
// We use --repository_cache instead of --output_user_root because:
// 1. output_user_root includes install/ directory (Bazel installation files)
// 2. If install/ is cached and restored, Bazel fails with "corrupt installation"
// 3. repository_cache caches downloaded external dependencies (safe to cache)
// Note: We intentionally do NOT use --disk_cache because:
// - If users have remote build cache enabled, disk_cache would be redundant
// - Remote cache already stores compiled outputs, so caching them again wastes storage
const bazelCacheDirName = ".bazel-cache"

type bazelPreparer struct{}

func newBazelPreparer() *bazelPreparer {
	return &bazelPreparer{}
}

func (*bazelPreparer) PrepareRepo(dir string) (string, error) {
	pathToCache := filepath.Join(dir, bazelCacheDirName)

	// Update .bazelrc with repository_cache and disk_cache options
	if err := appendToBazelrc(dir, pathToCache); err != nil {
		return "", err
	}

	// Update .bazelignore to exclude the cache directory from workspace
	// This prevents "bazel build //..." from trying to parse the cache directory
	if err := appendToBazelignore(dir, bazelCacheDirName); err != nil {
		return "", err
	}

	return pathToCache, nil
}

// appendToBazelrc adds the --repository_cache option to .bazelrc
func appendToBazelrc(dir, pathToCache string) error {
	fileName := filepath.Join(dir, ".bazelrc")

	// Use --repository_cache to cache downloaded external dependencies
	// This is safe to cache/restore across builds (unlike output_user_root which includes install/)
	// Note: We don't use --disk_cache because users may have remote build cache enabled,
	// and disk_cache would be redundant (remote cache already stores compiled outputs)
	cmdToOverrideRepo := "common --repository_cache=" + pathToCache + "\n"

	if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(fileName)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(cmdToOverrideRepo)
		return err
	}

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(cmdToOverrideRepo)
	return err
}

// appendToBazelignore adds cache directory to .bazelignore to exclude from workspace
func appendToBazelignore(dir, cacheDir string) error {
	fileName := filepath.Join(dir, ".bazelignore")
	ignoreEntry := cacheDir + "\n"

	if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(fileName)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(ignoreEntry)
		return err
	}

	// Check if cache dir is already in .bazelignore
	content, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	if strings.Contains(string(content), cacheDir) {
		return nil // Already present
	}

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(ignoreEntry)
	return err
}
