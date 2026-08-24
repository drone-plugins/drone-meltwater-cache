package autodetect

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	npmrcFileName = ".npmrc"

	// harnessNpmCacheDefault is a temporary Linux/Kubernetes contract. It must
	// match the Harness Run-step npm_config_cache injection and can be replaced
	// once the engine supplies an explicit cache path contract.
	harnessNpmCacheDefault = "/harness/.npm"
)

func npmCacheDirs(dir string, keyedFromLockfile bool) ([]string, error) {
	npmCache, err := resolveNpmTarballCache(dir)
	if err != nil {
		return nil, err
	}

	var dirs []string
	if keyedFromLockfile {
		dirs = append(dirs, filepath.Join(dir, nodeModulesDirName))
	}
	if npmCache != "" {
		dirs = append(dirs, npmCache)
	}
	return dirs, nil
}

type npmCacheSource struct {
	name              string
	value             func(string) (string, error)
	relativeToProject bool
}

var npmCacheSources = []npmCacheSource{
	{
		name:  "npm_config_cache",
		value: func(string) (string, error) { return os.Getenv("npm_config_cache"), nil },
	},
	{
		name:  "npm_config_cache",
		value: func(string) (string, error) { return os.Getenv("NPM_CONFIG_CACHE"), nil },
	},
	{
		name: ".npmrc cache",
		value: func(dir string) (string, error) {
			return npmrcCacheValue(filepath.Join(dir, npmrcFileName))
		},
		relativeToProject: true,
	},
	{
		name:  "Harness npm cache",
		value: func(string) (string, error) { return harnessNpmCacheDefault, nil },
	},
}

func resolveNpmTarballCache(dir string) (string, error) {
	for _, source := range npmCacheSources {
		path, err := source.value(dir)
		if err != nil {
			return "", err
		}
		if path == "" {
			continue
		}
		if source.relativeToProject && !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve %s path %q: %w", source.name, path, err)
		}
		return filepath.Clean(absPath), nil
	}
	return "", nil
}

func npmrcCacheValue(path string) (string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()

	var found string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(key), "cache") {
			continue
		}
		found = strings.Trim(strings.TrimSpace(val), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return found, nil
}
