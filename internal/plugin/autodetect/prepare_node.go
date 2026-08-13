package autodetect

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	npmCacheDirName = ".npm"
	npmrcFileName   = ".npmrc"
)

type nodePreparer struct{}

func newNodePreparer() *nodePreparer {
	return &nodePreparer{}
}

func (p *nodePreparer) PrepareRepo(dir string) (string, error) {
	dirs, err := p.PrepareRepoDirs(dir)
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("node preparer returned no cache directories")
	}
	return dirs[0], nil
}

// PrepareRepoDirs caches node_modules and npm's tarball cache, which npm ci can
// reuse after deleting node_modules. Cache precedence is environment, project
// .npmrc, then <dir>/.npm; the default is written to .npmrc for later npm runs.
func (*nodePreparer) PrepareRepoDirs(dir string) ([]string, error) {
	nodeModules := filepath.Join(dir, "node_modules")
	npmCache, err := resolveNpmTarballCache(dir)
	if err != nil {
		return nil, err
	}
	return []string{nodeModules, npmCache}, nil
}

func npmCacheFromEnv() string {
	if v := os.Getenv("npm_config_cache"); v != "" {
		return v
	}
	return os.Getenv("NPM_CONFIG_CACHE")
}

func resolveNpmTarballCache(dir string) (string, error) {
	if env := npmCacheFromEnv(); env != "" {
		absPath, err := filepath.Abs(env)
		if err != nil {
			return "", fmt.Errorf("failed to resolve npm_config_cache path %q: %w", env, err)
		}
		return filepath.Clean(absPath), nil
	}

	npmrcPath := filepath.Join(dir, npmrcFileName)
	if existing, err := npmrcCacheValue(npmrcPath); err != nil {
		return "", err
	} else if existing != "" {
		if !filepath.IsAbs(existing) {
			existing = filepath.Join(dir, existing)
		}
		absPath, err := filepath.Abs(existing)
		if err != nil {
			return "", fmt.Errorf("failed to resolve .npmrc cache path %q: %w", existing, err)
		}
		return filepath.Clean(absPath), nil
	}

	pathToCache := filepath.Join(dir, npmCacheDirName)
	if err := writeNpmrcCache(npmrcPath, pathToCache); err != nil {
		return "", err
	}
	return pathToCache, nil
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

func writeNpmrcCache(npmrcPath, cacheDir string) error {
	line := fmt.Sprintf("\ncache=%s\n", cacheDir)
	f, err := os.OpenFile(npmrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}
