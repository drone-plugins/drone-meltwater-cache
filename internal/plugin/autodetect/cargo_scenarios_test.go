package autodetect

import (
	"crypto/md5" // #nosec -- matches the existing cache key contract
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func setupCargoScenario(t *testing.T, files map[string]string) string {
	t.Helper()
	cwd, err := os.Getwd()
	test.Ok(t, err)
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, name)
		test.Ok(t, os.MkdirAll(filepath.Dir(p), 0755))
		test.Ok(t, os.WriteFile(p, []byte(content), 0644))
	}
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { test.Ok(t, os.Chdir(cwd)) })
	return root
}
func cargoExpectedDirs(home string) []string {
	return []string{filepath.Join(home, "registry", "cache"), filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}
}
func TestScenario_Cargo_Discovery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		detected bool
	}{
		{"root", map[string]string{"Cargo.lock": "root", "Cargo.toml": "manifest"}, true},
		{"nested", map[string]string{"app/Cargo.lock": "nested"}, true},
		{"too-deep", map[string]string{"apps/app/Cargo.lock": "deep"}, false},
		{"manifest-only", map[string]string{"Cargo.toml": "manifest"}, false},
		{"root-wins", map[string]string{"Cargo.lock": "root", "app/Cargo.lock": "nested"}, true},
		{"multiple-nested", map[string]string{"a/Cargo.lock": "a", "long/Cargo.lock": "long"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateCargoEnv(t)
			root := setupCargoScenario(t, tc.files)
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if !tc.detected {
				test.Equals(t, 0, len(tools))
				test.Equals(t, "", key)
				return
			}
			test.Equals(t, []string{"cargo"}, tools)
			test.Equals(t, cargoExpectedDirs(filepath.Join(root, ".cache", "cargo")), dirs)
			test.Assert(t, regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(key), "MD5 shape")
			lock := tc.files["Cargo.lock"]
			if lock == "" {
				lock = tc.files["app/Cargo.lock"]
			}
			if lock == "" {
				lock = "a"
			}
			test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte(lock))), key)
		})
	}
}
func TestScenario_Cargo_KeyAndOverrides(t *testing.T) {
	isolateCargoEnv(t)
	root := setupCargoScenario(t, map[string]string{"Cargo.lock": "one"})
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("unrelated"), 0644))
	_, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, same)
	test.Ok(t, os.WriteFile(filepath.Join(root, "Cargo.lock"), []byte("two"), 0644))
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, first != changed, "lock change must invalidate")
	t.Setenv("CARGO_HOME", "custom")
	dirs, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, changed, same)
	test.Equals(t, cargoExpectedDirs(filepath.Join(root, "custom")), dirs)
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
func TestScenario_Cargo_LegacyMixedKey(t *testing.T) {
	isolateCargoEnv(t)
	t.Setenv("COMPOSER_CACHE_DIR", "")
	t.Setenv("COMPOSER_VENDOR_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")
	root := setupCargoScenario(t, map[string]string{"composer.lock": "legacy"})
	_, tools, baseline, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"composer"}, tools)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("legacy"))), baseline)
	test.Ok(t, os.WriteFile(filepath.Join(root, "Cargo.lock"), []byte("cargo"), 0644))
	_, tools, mixed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"cargo", "composer"}, tools)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("cargo")))+baseline, mixed)
}
