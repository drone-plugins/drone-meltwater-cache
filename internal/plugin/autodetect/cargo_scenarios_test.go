package autodetect

import (
	"crypto/md5" // #nosec
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func cargoWrite(t *testing.T, path, data string) {
	t.Helper()
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(data), 0644))
}
func cargoDigest(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) } // #nosec

func TestScenario_Cargo_Detection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		selected string
	}{
		{"root", map[string]string{"Cargo.lock": "root", "Cargo.toml": "manifest"}, "root"},
		{"nested", map[string]string{"app/Cargo.lock": "nested"}, "nested"},
		{"deep-unsupported", map[string]string{"a/b/Cargo.lock": "deep"}, ""},
		{"manifest-only", map[string]string{"Cargo.toml": "manifest"}, ""},
		{"root-wins", map[string]string{"Cargo.lock": "root", "app/Cargo.lock": "nested"}, "root"},
		{"shortest-nested", map[string]string{"a/Cargo.lock": "short", "longer/Cargo.lock": "long"}, "short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cargoTestWorkspace(t)
			for p, s := range tc.files {
				cargoWrite(t, p, s)
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if tc.selected == "" {
				test.Equals(t, 0, len(tools))
				test.Equals(t, 0, len(dirs))
				test.Equals(t, "", key)
				return
			}
			test.Equals(t, []string{"cargo"}, tools)
			test.Equals(t, cargoDigest(tc.selected), key)
			test.Equals(t, cargoExpectedPaths(filepath.Join(root, ".cache", "cargo")), dirs)
		})
	}
}

func TestScenario_Cargo_Keys(t *testing.T) {
	cargoTestWorkspace(t)
	cargoWrite(t, "Cargo.lock", "v1")
	_, _, k1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	cargoWrite(t, "Cargo.toml", "unrelated change")
	_, _, k2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, k1, k2)
	cargoWrite(t, "Cargo.lock", "v2")
	_, _, k3, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, cargoDigest("v2"), k3)
	test.Assert(t, k1 != k3, "lock must invalidate")
}

func TestScenario_Cargo_MixedLegacy(t *testing.T) {
	root := cargoTestWorkspace(t)
	cargoWrite(t, "go.mod", "module example.org/project\n")
	legacyDirs, legacyTools, legacyKey, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"golang"}, legacyTools)
	test.Equals(t, []string{".go"}, legacyDirs)
	test.Equals(t, cargoDigest("module example.org/project\n"), legacyKey)
	cargoWrite(t, "Cargo.lock", "cargo")
	dirs, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"golang", "cargo"}, tools)
	test.Equals(t, append(legacyDirs, cargoExpectedPaths(filepath.Join(root, ".cache", "cargo"))...), dirs)
	test.Equals(t, legacyKey+cargoDigest("cargo"), key)
}

func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	cargoTestWorkspace(t)
	cargoWrite(t, "Cargo.lock", "lock")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
