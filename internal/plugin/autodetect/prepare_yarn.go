package autodetect

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	yarnBerryRCFileName = ".yarnrc.yml"
	// Berry writes __metadata right after the generated header comment.
	yarnLockHeaderLines = 20
)

type yarnPreparer struct{}

func newYarnPreparer() *yarnPreparer {
	return &yarnPreparer{}
}

func (*yarnPreparer) PrepareRepo(dir string) (string, error) {
	if isYarnBerry(dir) {
		return yarnBerryCacheFolder(dir)
	}

	pathToCache := filepath.Join(dir, ".yarn")
	// for yarn 1.x
	err := prepareYarn(pathToCache, filepath.Join(dir, ".yarnrc"), "\n--cache-folder %s\n")
	if err != nil {
		return "", err
	}

	return pathToCache, nil
}

// yarnCacheDirs returns node_modules plus, for Yarn Berry, the install state
// that lets a warm `yarn install` skip resolution and linking.
func yarnCacheDirs(dir string) ([]string, error) {
	dirs := []string{filepath.Join(dir, "node_modules")}
	if !isYarnBerry(dir) {
		return dirs, nil
	}

	statePath, err := yamlTopLevelString(filepath.Join(dir, yarnBerryRCFileName), "installStatePath")
	if err != nil {
		return nil, err
	}
	if statePath == "" {
		statePath = filepath.Join(".yarn", "install-state.gz")
	}
	state, err := resolveRepoPath(dir, statePath)
	if err != nil {
		return nil, err
	}

	return append(dirs, state), nil
}

// yarnBerryCacheFolder resolves Berry's cache without writing .yarnrc.yml.
// Harness sets YARN_ENABLE_GLOBAL_CACHE=false so Berry uses this per-project
// folder; zero-install repos that commit .yarn/cache keep working.
func yarnBerryCacheFolder(dir string) (string, error) {
	if value := os.Getenv("YARN_CACHE_FOLDER"); strings.TrimSpace(value) != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, value)
	}

	configured, err := yamlTopLevelString(filepath.Join(dir, yarnBerryRCFileName), "cacheFolder")
	if err != nil {
		return "", err
	}
	if configured != "" {
		return resolveRepoPath(dir, configured)
	}

	return filepath.Join(dir, ".yarn", "cache"), nil
}

// isYarnBerry reports whether dir is a Yarn 2+ project: it has .yarnrc.yml,
// a v2+ lockfile, or a packageManager field pinning yarn@2 or later.
func isYarnBerry(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, yarnBerryRCFileName)); err == nil && !info.IsDir() {
		return true
	}

	return yarnLockHasMetadata(filepath.Join(dir, "yarn.lock")) || packageManagerIsYarnBerry(dir)
}

func yarnLockHasMetadata(fileName string) bool {
	f, err := os.Open(fileName)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < yarnLockHeaderLines && scanner.Scan(); i++ {
		if strings.HasPrefix(scanner.Text(), "__metadata:") {
			return true
		}
	}

	return false
}

func packageManagerIsYarnBerry(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}

	var manifest struct {
		PackageManager string `json:"packageManager"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return false
	}

	version, found := strings.CutPrefix(manifest.PackageManager, "yarn@")
	if !found {
		return false
	}
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)

	return err == nil && n >= 2
}

func prepareYarn(pathToCache string, fileToWrite string, contentToWrite string) error {
	cmdToOverrideRepo := fmt.Sprintf(contentToWrite, pathToCache)

	if _, err := os.Stat(fileToWrite); errors.Is(err, os.ErrNotExist) {
		f, err := os.Create(fileToWrite)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(cmdToOverrideRepo)

		if err != nil {
			return err
		}

		return nil
	}

	f, err := os.OpenFile(fileToWrite, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd

	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(cmdToOverrideRepo)

	if err != nil {
		return err
	}

	return nil
}
