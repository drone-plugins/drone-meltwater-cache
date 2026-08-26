package autodetect

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// npm's tarball cache, moved out of ~/.npm. The plugin runs in its own
// container, where $HOME is unset and would point at the wrong home anyway.
// The workspace is the only volume shared with the build step, same reason
// maven, gradle, yarn and bazel relocate theirs.
const npmCacheDirName = ".npm"

const npmrcFileName = ".npmrc"

type nodePreparer struct{}

func newNodePreparer() *nodePreparer {
	return &nodePreparer{}
}

func (*nodePreparer) PrepareRepo(dir string) (string, error) {
	// Best effort. .npmrc is sometimes a read-only mounted secret, and failing
	// to write it shouldn't fail the step - we just lose the tarball cache.
	_ = prepareNpmrc(dir)

	return filepath.Join(dir, "node_modules"), nil
}

// npmCacheDir reports where npm will actually put its tarball cache, following
// npm's precedence: npm_config_cache beats .npmrc. An empty path means the
// cache was never relocated, so there is nothing here worth archiving.
func npmCacheDir(dir string) (string, error) {
	if envCache := npmCacheFromEnv(); envCache != "" {
		absPath, err := filepath.Abs(envCache)
		if err != nil {
			return "", err
		}

		return filepath.Clean(absPath), nil
	}

	// PrepareRepo ran first, so an entry exists unless .npmrc was unreadable.
	configured, err := npmCacheFromNpmrc(filepath.Join(dir, npmrcFileName))
	if err != nil || configured == "" {
		return "", nil
	}

	if filepath.IsAbs(configured) {
		return filepath.Clean(configured), nil
	}

	return filepath.Join(dir, configured), nil
}

// nodeModulesDir is for tools that install into node_modules but keep their
// download cache elsewhere, e.g. yarn.
func nodeModulesDir(dir string) (string, error) {
	return filepath.Join(dir, "node_modules"), nil
}

// prepareNpmrc appends cache=<dir>/.npm to .npmrc. An existing entry wins,
// whether it is the user's or ours from an earlier step, so restore and save
// agree on the path and we never append twice. npm_config_cache overrides
// .npmrc anyway, so nothing is written when it is set.
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

// npmCacheFromNpmrc returns the last cache entry in the file, which is the one
// npm honours. A missing file yields an empty value.
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
