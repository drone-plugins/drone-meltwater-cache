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
	pathToCache := filepath.Join(dir, bazelCacheDirName)

	if err := appendToBazelrcBzlmod(dir, pathToCache); err != nil {
		return "", err
	}

	if err := appendToBazelignore(dir, bazelCacheDirName); err != nil {
		return "", err
	}

	return pathToCache, nil
}

// appendToBazelrcBzlmod adds the --repository_cache option to .bazelrc.
// This is a separate function to allow mocking in tests.
// For Bazel 9+ it also disables --repo_contents_cache (see isBazel9OrNewer).
func appendToBazelrcBzlmod(dir, pathToCache string) error {
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

	f, err := osOpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(cmdToOverrideRepo)
	return err
}
