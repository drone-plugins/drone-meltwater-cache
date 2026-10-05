package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func writeBundleConfigFile(t *testing.T, dir, contents string) string {
	t.Helper()

	test.Ok(t, os.MkdirAll(filepath.Join(dir, bundleConfigDir), 0755))
	path := filepath.Join(dir, bundleConfigDir, bundleConfigFile)
	test.Ok(t, os.WriteFile(path, []byte(contents), 0644))

	return path
}

func TestFastlanePreparerFreshRepo(t *testing.T) {
	dir := t.TempDir()

	path, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, "vendor", "bundle"), path)

	data, err := os.ReadFile(filepath.Join(dir, bundleConfigDir, bundleConfigFile))
	test.Ok(t, err)
	test.Equals(t, "---\nBUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

// Restore and save both run PrepareRepo, so a second call must not append again.
func TestFastlanePreparerIdempotent(t *testing.T) {
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

func TestFastlanePreparerPreservesExistingSettings(t *testing.T) {
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
	dir := t.TempDir()
	configPath := writeBundleConfigFile(t, dir, "---\nBUNDLE_JOBS: \"4\"")

	_, err := newFastlanePreparer().PrepareRepo(dir)
	test.Ok(t, err)

	data, err := os.ReadFile(configPath)
	test.Ok(t, err)
	test.Equals(t, "---\nBUNDLE_JOBS: \"4\"\nBUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

func TestFastlanePreparerRespectsExistingBundlePath(t *testing.T) {
	for name, contents := range map[string]string{
		"doubleQuoted": "---\nBUNDLE_PATH: \"custom/gems\"\n",
		"unquoted":     "---\nBUNDLE_PATH: custom/gems\n",
		"singleQuoted": "---\nBUNDLE_PATH: 'custom/gems'\n",
		"extraSpacing": "---\nBUNDLE_PATH:    \"custom/gems\"   \n",
	} {
		t.Run(name, func(t *testing.T) {
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
