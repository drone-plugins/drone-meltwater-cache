package autodetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	bundleConfigDir  = ".bundle"
	bundleConfigFile = "config"
	bundleVendorPath = "vendor/bundle"
)

// bundlePathLineRe matches a BUNDLE_PATH entry in a Bundler config file.
// Bundler itself always writes `KEY: "value"`, but these files are routinely
// hand-edited and committed, so unquoted and single-quoted values are matched
// too rather than being mistaken for "not configured".
var bundlePathLineRe = regexp.MustCompile(`^BUNDLE_PATH\s*:\s*(.*?)\s*$`)

type fastlanePreparer struct{}

func newFastlanePreparer() *fastlanePreparer {
	return &fastlanePreparer{}
}

// PrepareRepo redirects Bundler's gem install path to vendor/bundle so it can be
// cached, and reports the directory Bundler will actually install into.
//
// Bundler resolves settings in the order local config -> environment -> global
// config -> default, so the repo's own .bundle/config wins over BUNDLE_PATH.
// This is the opposite of NuGet/NUGET_PACKAGES in prepare_dotnet.go; mirroring
// that env-var-first shape here would make us cache a directory Bundler never
// installs into whenever a repo sets both.
func (*fastlanePreparer) PrepareRepo(dir string) (string, error) {
	configPath := bundleConfigPath(dir)

	data, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	if existing, ok := findBundlePath(data); ok {
		return resolveBundlePath(dir, existing)
	}

	if envPath := os.Getenv("BUNDLE_PATH"); envPath != "" {
		return resolveBundlePath(dir, envPath)
	}

	globalPath, ok, err := globalBundlePath()
	if err != nil {
		return "", err
	}

	if ok {
		return resolveBundlePath(dir, globalPath)
	}

	if err := appendBundlePath(configPath, data); err != nil {
		return "", err
	}

	return filepath.Join(dir, "vendor", "bundle"), nil
}

// bundleConfigPath returns the file Bundler reads local settings from.
// BUNDLE_APP_CONFIG relocates that file (Bundler expands it relative to the
// bundle root), so writing to .bundle/config when it is set would silently have
// no effect on the build.
func bundleConfigPath(dir string) string {
	appConfig := os.Getenv("BUNDLE_APP_CONFIG")
	if appConfig == "" {
		return filepath.Join(dir, bundleConfigDir, bundleConfigFile)
	}

	if !filepath.IsAbs(appConfig) {
		appConfig = filepath.Join(dir, appConfig)
	}

	return filepath.Join(appConfig, bundleConfigFile)
}

func globalBundlePath() (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}

	data, err := os.ReadFile(filepath.Join(home, bundleConfigDir, bundleConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}

	if err != nil {
		return "", false, err
	}

	value, ok := findBundlePath(data)

	return value, ok, nil
}

// findBundlePath reports the configured BUNDLE_PATH, if any. Only this one key
// is parsed: the rest of the file is never interpreted, so mirror credentials,
// comments and settings this plugin does not understand cannot be lost.
func findBundlePath(data []byte) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		match := bundlePathLineRe.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}

		if value := unquoteYAMLScalar(match[1]); value != "" {
			return value, true
		}
	}

	return "", false
}

func unquoteYAMLScalar(value string) string {
	const minQuoted = 2
	if len(value) >= minQuoted {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}

	return value
}

// appendBundlePath adds a BUNDLE_PATH entry while preserving every existing byte
// of the file. Appending rather than reserializing keeps comments, unquoted
// values and keys containing ':' or '/' (e.g. BUNDLE_MIRROR__HTTPS://RUBYGEMS__ORG/)
// intact. It stays idempotent because a later call finds the key and returns
// early, so this never degenerates into the repeated-append corruption that hit
// gradle.properties (CI-24154).
func appendBundlePath(path string, existing []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gomnd
		return err
	}

	var b strings.Builder

	if len(existing) == 0 {
		b.WriteString("---\n")
	} else {
		b.Write(existing)

		if !strings.HasSuffix(string(existing), "\n") {
			b.WriteString("\n")
		}
	}

	fmt.Fprintf(&b, "BUNDLE_PATH: %q\n", bundleVendorPath)

	return os.WriteFile(path, []byte(b.String()), 0644) //nolint:gomnd
}

// resolveBundlePath resolves a configured path the way Bundler does: relative
// values are anchored to the bundle root, not the process working directory.
func resolveBundlePath(dir, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve bundle path %q: %w", path, err)
	}

	return filepath.Clean(absPath), nil
}
