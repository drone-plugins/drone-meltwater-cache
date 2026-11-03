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
func cargoDigest(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }

func TestScenario_Cargo_Detection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		selected string
	}{
		{"root", map[string]string{"Cargo.lock": "root", "Cargo.toml": "manifest"}, "root"},
		{"nested", map[string]string{"app/Cargo.lock": "nested"}, "nested"},
		{"root-precedence", map[string]string{"Cargo.lock": "root", "app/Cargo.lock": "nested"}, "root"},
		{"shortest", map[string]string{"a/Cargo.lock": "short", "longer/Cargo.lock": "long"}, "short"},
		{"deep-unsupported", map[string]string{"a/b/Cargo.lock": "deep"}, ""},
		{"manifest-only", map[string]string{"Cargo.toml": "manifest"}, ""},
		{"empty", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cargoTestWorkspace(t)
			for path, content := range tc.files {
				cargoWrite(t, path, content)
			}
			for i := 0; i < 2; i++ {
				dirs, tools, key, err := DetectDirectoriesToCache(false)
				test.Ok(t, err)
				if tc.selected == "" {
					test.Equals(t, 0, len(dirs))
					test.Equals(t, 0, len(tools))
					test.Equals(t, "", key)
					continue
				}
				test.Equals(t, []string{"cargo"}, tools)
				test.Equals(t, cargoDigest(tc.selected), key)
				test.Equals(t, cargoExpectedDirs(filepath.Join(root, ".cache", "cargo")), dirs)
			}
		})
	}
}

func TestScenario_Cargo_KeyInvalidation(t *testing.T) {
	cargoTestWorkspace(t)
	cargoWrite(t, "Cargo.lock", "v1")
	dirs, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	cargoWrite(t, "unrelated.txt", "source change")
	cargoWrite(t, "Cargo.toml", "manifest change")
	dirs2, tools2, key2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, dirs, dirs2)
	test.Equals(t, tools, tools2)
	test.Equals(t, key, key2)
	cargoWrite(t, "Cargo.lock", "v2")
	_, _, key2, err = DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, cargoDigest("v2"), key2)
	test.Assert(t, key != key2, "lockfile must invalidate")
}

func TestScenario_Cargo_LegacyCoexistence(t *testing.T) {
	cargoTestWorkspace(t)
	t.Setenv("GOCACHE", "")
	cargoWrite(t, "go.mod", "module example.com/fixture\n")
	legacyDirs, legacyTools, legacyKey, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"golang"}, legacyTools)
	test.Equals(t, cargoDigest("module example.com/fixture\n"), legacyKey)
	cargoWrite(t, "Cargo.lock", "cargo")
	dirs, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"golang", "cargo"}, tools)
	test.Equals(t, legacyKey+cargoDigest("cargo"), key)
	test.Equals(t, legacyDirs, dirs[:len(legacyDirs)])
	test.Ok(t, os.Remove("Cargo.lock"))
	dirs, tools, key, err = DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, legacyDirs, dirs)
	test.Equals(t, legacyTools, tools)
	test.Equals(t, legacyKey, key)
}

func TestScenario_Cargo_Bypass(t *testing.T) {
	cargoTestWorkspace(t)
	cargoWrite(t, "Cargo.lock", "locked")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
