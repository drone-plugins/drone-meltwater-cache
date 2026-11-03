package autodetect

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func cargoWrite(t *testing.T, path, content string) {
	t.Helper()
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(content), 0644))
}
func cargoMD5(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }

func TestScenario_Cargo_Detection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		detected bool
		content  string
	}{
		{"root", map[string]string{"Cargo.lock": "root"}, true, "root"},
		{"nested", map[string]string{"app/Cargo.lock": "nested"}, true, "nested"},
		{"deep", map[string]string{"a/b/Cargo.lock": "deep"}, false, ""},
		{"manifest", map[string]string{"Cargo.toml": "[package]"}, false, ""},
		{"root wins", map[string]string{"Cargo.lock": "root", "app/Cargo.lock": "nested"}, true, "root"},
		{"two siblings", map[string]string{"a/Cargo.lock": "a", "longer/Cargo.lock": "b"}, true, "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cargoTestWorkspace(t)
			for path, body := range tc.files {
				cargoWrite(t, path, body)
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if tc.detected {
				test.Equals(t, []string{"cargo"}, tools)
				test.Equals(t, cargoExpectedPaths(filepath.Join(root, ".cache", "cargo")), dirs)
				test.Equals(t, cargoMD5(tc.content), key)
			} else {
				test.Assert(t, len(dirs) == 0 && len(tools) == 0 && key == "", "unexpected detection")
			}
		})
	}
}

func TestScenario_Cargo_KeyInvalidation(t *testing.T) {
	cargoTestWorkspace(t)
	cargoWrite(t, "Cargo.lock", "v1")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	cargoWrite(t, "Cargo.toml", "unrelated manifest edit")
	cargoWrite(t, "src/main.rs", "fn main() {}")
	_, _, second, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, second)
	cargoWrite(t, "Cargo.lock", "v2")
	_, _, third, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, cargoMD5("v2"), third)
	test.Assert(t, first != third, "lockfile change must invalidate")
}

func TestScenario_Cargo_MixedLegacy(t *testing.T) {
	root := cargoTestWorkspace(t)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	cargoWrite(t, "package-lock.json", "node lock")
	dirs, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"node"}, tools)
	test.Equals(t, []string{filepath.Join(root, "node_modules")}, dirs)
	test.Equals(t, cargoMD5("node lock"), key)
	cargoWrite(t, "Cargo.lock", "cargo lock")
	mixedDirs, mixedTools, mixedKey, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"node", "cargo"}, mixedTools)
	test.Equals(t, append(dirs, cargoExpectedPaths(filepath.Join(root, ".cache", "cargo"))...), mixedDirs)
	test.Equals(t, key+cargoMD5("cargo lock"), mixedKey)
}

func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	cargoTestWorkspace(t)
	cargoWrite(t, "Cargo.lock", "locked")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Assert(t, len(dirs) == 0 && len(tools) == 0 && key == "", "custom mount must bypass preparation")
}
