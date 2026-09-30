package autodetect

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// npmCacheDirName is npm's default cache directory name (~/.npm).
const npmCacheDirName = ".npm"

const npmrcFileName = ".npmrc"

type nodePreparer struct{}

func newNodePreparer() *nodePreparer {
	return &nodePreparer{}
}

func (*nodePreparer) PrepareRepo(dir string) (string, error) {
	// node_modules lives on the shared workspace volume, so later npm commands
	// pick it up with no config change. Do not write .npmrc: on Harness the
	// checkout root is /harness, and appending cache=/harness/.npm dirties a
	// tracked project config. `npm version` then fails its clean-tree check.
	return filepath.Join(dir, "node_modules"), nil
}

type nodeFallbackPreparer struct{}

func newNodeFallbackPreparer() *nodeFallbackPreparer {
	return &nodeFallbackPreparer{}
}

func (*nodeFallbackPreparer) PrepareRepo(dir string) (string, error) {
	return filepath.Join(dir, "node_modules"), nil
}

// npmCacheDirs resolves the npm tarball cache when npm is already pointed at
// one. It never creates or rewrites an npmrc.
func npmCacheDirs(dir string) ([]string, error) {
	cacheDir, err := effectiveNpmCacheDir(dir)
	if err != nil || cacheDir == "" {
		return nil, err
	}

	return []string{cacheDir}, nil
}

// nodeModulesDirs resolves the node_modules path.
func nodeModulesDirs(dir string) ([]string, error) {
	return []string{filepath.Join(dir, "node_modules")}, nil
}

// effectiveNpmCacheDir follows npm's precedence for the cache location:
// environment, then the project .npmrc, then the built-in ~/.npm default.
// The default is included only when it already sits on the shared workspace
// (HOME is the checkout). Relocating ~/.npm by writing .npmrc is intentionally
// not done.
func effectiveNpmCacheDir(dir string) (string, error) {
	if envCache := npmCacheFromEnv(); envCache != "" {
		absPath, err := filepath.Abs(envCache)
		if err != nil {
			return "", err
		}

		return filepath.Clean(absPath), nil
	}

	configured, err := npmCacheFromNpmrc(filepath.Join(dir, npmrcFileName))
	if err != nil {
		return "", err
	}

	if configured != "" {
		if filepath.IsAbs(configured) {
			return filepath.Clean(configured), nil
		}

		return filepath.Join(dir, configured), nil
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", nil
	}

	defaultCache := filepath.Clean(filepath.Join(home, npmCacheDirName))
	root := workspaceRoot(dir)
	if root == "" || !pathWithin(defaultCache, root) {
		return "", nil
	}

	return defaultCache, nil
}

func workspaceRoot(dir string) string {
	if ws := strings.TrimSpace(os.Getenv("HARNESS_WORKSPACE")); ws != "" {
		abs, err := filepath.Abs(ws)
		if err == nil {
			return filepath.Clean(abs)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(dir)
	}

	return filepath.Clean(wd)
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func npmCacheFromEnv() string {
	// npm accepts either casing.
	for _, name := range []string{"npm_config_cache", "NPM_CONFIG_CACHE"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}

	return ""
}

// npmCacheFromNpmrc returns the cache entry from .npmrc.
func npmCacheFromNpmrc(fileName string) (string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		return "", err
	}
	defer f.Close()

	var value string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		key, rest, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "cache" {
			continue
		}

		value = strings.Trim(strings.TrimSpace(rest), `"'`)
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return value, nil
}
