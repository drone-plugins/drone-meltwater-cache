package autodetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type bzlmodPreparer struct{}

func newBzlmodPreparer() *bzlmodPreparer {
	return &bzlmodPreparer{}
}

func (*bzlmodPreparer) PrepareRepo(dir string) (string, error) {
	fileName := filepath.Join(dir, ".bazelrc")
	pathToCache := filepath.Join(dir, ".bazel")

	runtimeCache := filepath.Join(pathToCache, "run")
	moduleCache := filepath.Join(pathToCache, "module")
	registryCache := filepath.Join(pathToCache, "registry")

	cmdToOverrideRepo := fmt.Sprintf(`
	build --test_tmpdir=%s
	test --test_tmpdir=%s
	build --module_cache=%s
	build --registry_cache=%s
	`,
		runtimeCache,
		runtimeCache,
		moduleCache,
		registryCache,
	)

	if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(fileName)
		if err != nil {
			return "", err
		}
		defer f.Close()
		_, err = f.WriteString(cmdToOverrideRepo)

		if err != nil {
			return "", err
		}

		return pathToCache, nil
	}

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd

	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.WriteString(cmdToOverrideRepo)

	if err != nil {
		return "", err
	}

	return pathToCache, nil
}
