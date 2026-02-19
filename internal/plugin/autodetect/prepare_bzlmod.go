package autodetect

import (
	"errors"
	"os"
	"path/filepath"
)

// Variable for testing - allows mocking os.OpenFile
var osOpenFile = os.OpenFile

type bzlmodPreparer struct{}

func newBzlmodPreparer() *bzlmodPreparer {
	return &bzlmodPreparer{}
}

func (*bzlmodPreparer) PrepareRepo(dir string) (string, error) {
	// Use same cache directory as bazelPreparer
	pathToCache := filepath.Join(dir, bazelCacheDirName)

	// Update .bazelrc with repository_cache and disk_cache options
	if err := appendToBazelrcBzlmod(dir, pathToCache); err != nil {
		return "", err
	}

	// Update .bazelignore to exclude the cache directory from workspace
	// This prevents "bazel build //..." from trying to parse the cache directory
	if err := appendToBazelignore(dir, bazelCacheDirName); err != nil {
		return "", err
	}

	return pathToCache, nil
}

// appendToBazelrcBzlmod adds the --repository_cache option to .bazelrc
// This is a separate function to allow mocking in tests
func appendToBazelrcBzlmod(dir, pathToCache string) error {
	fileName := filepath.Join(dir, ".bazelrc")

	// Use --repository_cache to cache downloaded external dependencies
	// This is safe to cache/restore across builds (unlike output_user_root which includes install/)
	// Note: We don't use --disk_cache because users may have remote build cache enabled,
	// and disk_cache would be redundant (remote cache already stores compiled outputs)
	// We disable --repo_contents_cache because in Bazel 9+ it defaults to inside repository_cache,
	// and when that's inside the workspace, Bazel fails with "repo contents cache inside main repo" error
	cmdToOverrideRepo := "common --repository_cache=" + pathToCache + "\n" +
		"common --repo_contents_cache=\n"

	if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(fileName)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(cmdToOverrideRepo)
		return err
	}

	f, err := osOpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(cmdToOverrideRepo)
	return err
}
