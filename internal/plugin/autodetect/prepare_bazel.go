package autodetect

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// bazelCacheDirName is the cache directory for Bazel.
// Kept inside the workspace (shared volume) so that save/restore cache steps
// running in separate containers can access it.
//
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

	if err := appendToBazelrc(dir, pathToCache); err != nil {
		return "", err
	}

	if err := appendToBazelignore(dir, bazelCacheDirName); err != nil {
		return "", err
	}

	return pathToCache, nil
}

// isBazel9OrNewer checks both USE_BAZEL_VERSION env var (takes precedence with
// bazelisk) and .bazelversion file. Returns true only when the major version is
// confirmed to be >= 9. When the version cannot be determined the safe default
// is false, which avoids adding --repo_contents_cache= that older versions
// reject as "Unrecognized option".
func isBazel9OrNewer(dir string) bool {
	version := os.Getenv("USE_BAZEL_VERSION")

	if version == "" {
		data, err := os.ReadFile(filepath.Join(dir, ".bazelversion"))
		if err != nil {
			return false
		}
		version = string(data)
	}

	version = strings.TrimSpace(version)
	parts := strings.SplitN(version, ".", 2)
	if len(parts) == 0 {
		return false
	}
	major, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return false
	}
	return major >= 9
}

// appendToBazelrc adds the --repository_cache option to .bazelrc.
// For Bazel 9+ it also disables --repo_contents_cache which otherwise defaults
// to <repository_cache>/contents and fails when that sits inside the workspace.
func appendToBazelrc(dir, pathToCache string) error {
	fileName := filepath.Join(dir, ".bazelrc")

	cmdToOverrideRepo := "common --repository_cache=" + pathToCache + "\n"
	if isBazel9OrNewer(dir) {
		cmdToOverrideRepo += "common --repo_contents_cache=\n"
	}

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

// appendToBazelignore adds cache directory to .bazelignore to exclude from workspace.
// This prevents "bazel build //..." from trying to parse the cache directory.
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

	content, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	if strings.Contains(string(content), cacheDir) {
		return nil
	}

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(ignoreEntry)
	return err
}
