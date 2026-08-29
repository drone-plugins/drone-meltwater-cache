package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

// isolateBundlerEnv points HOME at an empty directory and clears Bundler's env
// vars, so these tests can never pick up the developer's real ~/.bundle/config.
// t.Setenv registers the restore, so unsetting afterwards is still reverted.
func isolateBundlerEnv(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, key := range []string{"BUNDLE_PATH", "BUNDLE_APP_CONFIG"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func writeBundleConfigFile(t *testing.T, dir, contents string) string {
	t.Helper()

	test.Ok(t, os.MkdirAll(filepath.Join(dir, bundleConfigDir), 0755))
	path := filepath.Join(dir, bundleConfigDir, bundleConfigFile)
	test.Ok(t, os.WriteFile(path, []byte(contents), 0644))

	return path
}

func TestFastlanePreparerFreshRepo(t *testing.T) {
	isolateBundlerEnv(t)

	dir := t.TempDir()

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "vendor", "bundle"), path)

	data, err := os.ReadFile(filepath.Join(dir, bundleConfigDir, bundleConfigFile))
	test.Ok(t, err)
	test.Equals(t, "---\nBUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

func TestFastlanePreparerIdempotent(t *testing.T) {
	isolateBundlerEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, bundleConfigDir, bundleConfigFile)

	_, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)

	first, err := os.ReadFile(configPath)
	test.Ok(t, err)

	_, err = newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)

	second, err := os.ReadFile(configPath)
	test.Ok(t, err)

	test.Equals(t, string(first), string(second))
}

// TestFastlanePreparerPreservesUnparseableSettings is the regression test for the
// reserialize-the-whole-file bug: a private gem mirror, BUNDLE_FROZEN, comments
// and unquoted values were all silently dropped. Every original byte must survive.
func TestFastlanePreparerPreservesUnparseableSettings(t *testing.T) {
	isolateBundlerEnv(t)

	// Real `bundle config --local` output, plus hand-written but legal YAML.
	original := `---
# internal mirror: required, we are air-gapped
BUNDLE_MIRROR__HTTPS://RUBYGEMS__ORG/: "https://mirror.internal"
BUNDLE_WITHOUT: "development:test"
BUNDLE_JOBS: 4
BUNDLE_FROZEN: 'true'
`

	dir := t.TempDir()
	configPath := writeBundleConfigFile(t, dir, original)

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "vendor", "bundle"), path)

	data, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Equals(t, original+"BUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

func TestFastlanePreparerAppendsNewlineWhenMissing(t *testing.T) {
	isolateBundlerEnv(t)

	dir := t.TempDir()
	configPath := writeBundleConfigFile(t, dir, "---\nBUNDLE_JOBS: \"4\"")

	_, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)

	data, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Equals(t, "---\nBUNDLE_JOBS: \"4\"\nBUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

// TestFastlanePreparerLocalConfigBeatsEnvVar pins Bundler's actual precedence:
// local .bundle/config wins over BUNDLE_PATH. This is the opposite of
// NuGet/NUGET_PACKAGES, and getting it backwards means caching a directory
// Bundler never installs into.
func TestFastlanePreparerLocalConfigBeatsEnvVar(t *testing.T) {
	isolateBundlerEnv(t)
	t.Setenv("BUNDLE_PATH", "/from/env")

	dir := t.TempDir()
	original := "---\nBUNDLE_PATH: \"custom/gems\"\n"
	configPath := writeBundleConfigFile(t, dir, original)

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "custom", "gems"), path)

	data, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Equals(t, original, string(data))
}

func TestFastlanePreparerUsesEnvVarWhenNoLocalConfig(t *testing.T) {
	isolateBundlerEnv(t)
	t.Setenv("BUNDLE_PATH", "/custom/bundle/cache")

	dir := t.TempDir()

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, "/custom/bundle/cache", path)

	_, err = os.Stat(filepath.Join(dir, bundleConfigDir, bundleConfigFile))
	test.Assert(t, os.IsNotExist(err), "expected no config to be written, got err=%v", err)
}

func TestFastlanePreparerEnvVarRelativeToBundleRoot(t *testing.T) {
	isolateBundlerEnv(t)
	t.Setenv("BUNDLE_PATH", "relative/cache")

	dir := t.TempDir()

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "relative", "cache"), path)
}

func TestFastlanePreparerReadsUnquotedAndSingleQuotedValues(t *testing.T) {
	for name, contents := range map[string]string{
		"unquoted":     "---\nBUNDLE_PATH: custom/gems\n",
		"singleQuoted": "---\nBUNDLE_PATH: 'custom/gems'\n",
		"extraSpacing": "---\nBUNDLE_PATH:    \"custom/gems\"   \n",
	} {
		t.Run(name, func(t *testing.T) {
			isolateBundlerEnv(t)

			dir := t.TempDir()
			configPath := writeBundleConfigFile(t, dir, contents)

			path, err := newFastlanePreparer().PrepareRepo(dir)
			test.Ok(t, err)
			test.Equals(t, filepath.Join(dir, "custom", "gems"), path)

			data, err := os.ReadFile(configPath)
			test.Ok(t, err)
			test.Equals(t, contents, string(data))
		})
	}
}

func TestFastlanePreparerIgnoresCommentedOutBundlePath(t *testing.T) {
	isolateBundlerEnv(t)

	dir := t.TempDir()
	original := "---\n# BUNDLE_PATH: \"commented/out\"\n"
	configPath := writeBundleConfigFile(t, dir, original)

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "vendor", "bundle"), path)

	data, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Equals(t, original+"BUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

// TestFastlanePreparerHonorsBundleAppConfig covers BUNDLE_APP_CONFIG relocating
// the local config: writing .bundle/config while it is set would have no effect
// on the build at all.
func TestFastlanePreparerHonorsBundleAppConfig(t *testing.T) {
	isolateBundlerEnv(t)

	dir := t.TempDir()
	appConfig := t.TempDir()
	t.Setenv("BUNDLE_APP_CONFIG", appConfig)

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "vendor", "bundle"), path)

	data, err := os.ReadFile(filepath.Join(appConfig, bundleConfigFile))
	test.Ok(t, err)
	test.Equals(t, "---\nBUNDLE_PATH: \"vendor/bundle\"\n", string(data))

	_, err = os.Stat(filepath.Join(dir, bundleConfigDir, bundleConfigFile))
	test.Assert(t, os.IsNotExist(err), "expected no .bundle/config, got err=%v", err)
}

func TestFastlanePreparerBundleAppConfigRelativeToBundleRoot(t *testing.T) {
	isolateBundlerEnv(t)
	t.Setenv("BUNDLE_APP_CONFIG", "cfg")

	dir := t.TempDir()

	_, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "cfg", bundleConfigFile))
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(data), "vendor/bundle"), "got %q", string(data))
}

func TestFastlanePreparerReadsGlobalConfigWhenNoLocal(t *testing.T) {
	isolateBundlerEnv(t)

	home, err := os.UserHomeDir()
	test.Ok(t, err)
	test.Ok(t, os.MkdirAll(filepath.Join(home, bundleConfigDir), 0755))
	test.Ok(t, os.WriteFile(
		filepath.Join(home, bundleConfigDir, bundleConfigFile),
		[]byte("---\nBUNDLE_PATH: \"/opt/global/gems\"\n"), 0644))

	dir := t.TempDir()

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, "/opt/global/gems", path)

	// Writing a local config would override the customer's global setting.
	_, err = os.Stat(filepath.Join(dir, bundleConfigDir, bundleConfigFile))
	test.Assert(t, os.IsNotExist(err), "expected no local config, got err=%v", err)
}

func TestFastlanePreparerLocalConfigBeatsGlobalConfig(t *testing.T) {
	isolateBundlerEnv(t)

	home, err := os.UserHomeDir()
	test.Ok(t, err)
	test.Ok(t, os.MkdirAll(filepath.Join(home, bundleConfigDir), 0755))
	test.Ok(t, os.WriteFile(
		filepath.Join(home, bundleConfigDir, bundleConfigFile),
		[]byte("---\nBUNDLE_PATH: \"/opt/global/gems\"\n"), 0644))

	dir := t.TempDir()
	writeBundleConfigFile(t, dir, "---\nBUNDLE_PATH: \"local/gems\"\n")

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "local", "gems"), path)
}
