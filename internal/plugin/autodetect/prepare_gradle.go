package autodetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type gradlePreparer struct{}

func newGradlePreparer() *gradlePreparer {
	return &gradlePreparer{}
}

func (*gradlePreparer) PrepareRepo(dir string) (string, error) {
	fileName := filepath.Join(dir, "gradle.properties")

	// Use project directory + .gradle as the cache location
	// This ensures Gradle and the plugin use the same path
	// e.g., if dir=/harness, pathToCache=/harness/.gradle
	pathToCache := filepath.Join(dir, ".gradle")
	cmdToOverrideRepo := fmt.Sprintf("systemProp.gradle.user.home=%s\norg.gradle.caching=true\n", pathToCache)

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

	// If the file is non-empty and does not end with a newline, prepend one so
	// appended properties are not concatenated onto the last customer line (CI-24154).
	info, err := os.Stat(fileName)
	if err != nil {
		return "", err
	}
	if info.Size() > 0 {
		f, err := os.Open(fileName)
		if err != nil {
			return "", err
		}
		buf := make([]byte, 1)
		_, err = f.ReadAt(buf, info.Size()-1)
		f.Close()
		if err != nil {
			return "", err
		}
		if buf[0] != '\n' {
			cmdToOverrideRepo = "\n" + cmdToOverrideRepo
		}
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
