package autodetect

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// npmCacheDirName is the workspace directory for relocated npm tarball cache.
const npmCacheDirName = ".npm"

const npmrcFileName = ".npmrc"

type nodePreparer struct{}

func newNodePreparer() *nodePreparer {
	return &nodePreparer{}
}

func (*nodePreparer) PrepareRepo(dir string) (string, error) {
	// Best-effort configuration of .npmrc.
	_ = prepareNpmrc(dir)

	return filepath.Join(dir, "node_modules"), nil
}

type nodeFallbackPreparer struct{}

func newNodeFallbackPreparer() *nodeFallbackPreparer {
	return &nodeFallbackPreparer{}
}

func (*nodeFallbackPreparer) PrepareRepo(dir string) (string, error) {
	return filepath.Join(dir, "node_modules"), nil
}

// npmCacheDirs resolves the effective npm cache directory.
func npmCacheDirs(dir string) ([]string, error) {
	if envCache := npmCacheFromEnv(); envCache != "" {
		absPath, err := filepath.Abs(envCache)
		if err != nil {
			return nil, err
		}

		return []string{filepath.Clean(absPath)}, nil
	}

	configured, err := npmCacheFromNpmrc(filepath.Join(dir, npmrcFileName))
	if err != nil || configured == "" {
		return nil, nil
	}

	if filepath.IsAbs(configured) {
		return []string{filepath.Clean(configured)}, nil
	}

	return []string{filepath.Join(dir, configured)}, nil
}

// nodeModulesDirs resolves the node_modules path.
func nodeModulesDirs(dir string) ([]string, error) {
	return []string{filepath.Join(dir, "node_modules")}, nil
}

// prepareNpmrc appends cache=<dir>/.npm to .npmrc if not already configured.
func prepareNpmrc(dir string) error {
	if npmCacheFromEnv() != "" {
		return nil
	}

	fileName := filepath.Join(dir, npmrcFileName)

	configured, err := npmCacheFromNpmrc(fileName)
	if err != nil {
		return err
	}

	if configured != "" {
		return nil
	}

	cacheEntry := "cache=" + filepath.Join(dir, npmCacheDirName) + "\n"

	info, err := os.Stat(fileName)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		return os.WriteFile(fileName, []byte(cacheEntry), 0644) //nolint:gomnd
	}

	// Don't run our entry onto the end of an unterminated last line (CI-24154).
	if info.Size() > 0 {
		terminated, err := endsWithNewline(fileName, info.Size())
		if err != nil {
			return err
		}

		if !terminated {
			cacheEntry = "\n" + cacheEntry
		}
	}

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gomnd
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(cacheEntry)

	return err
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

func endsWithNewline(fileName string, size int64) (bool, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, size-1); err != nil {
		return false, err
	}

	return buf[0] == '\n', nil
}
