package autodetect

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func cargoWrite(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, name)
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(data), 0644))
}
func cargoPaths(home string) []string {
	return []string{filepath.Join(home, "registry", "index"), filepath.Join(home, "registry", "cache"), filepath.Join(home, "git", "db")}
}
func TestScenario_Cargo_Layouts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    []string
		detected bool
	}{
		{"root", []string{"Cargo.lock", "Cargo.toml"}, true},
		{"nested", []string{"app/Cargo.lock"}, true},
		{"deep", []string{"apps/app/Cargo.lock"}, false},
		{"lockless", []string{"Cargo.toml"}, false},
		{"multiple", []string{"Cargo.lock", "app/Cargo.lock"}, true},
		{"siblings", []string{"a/Cargo.lock", "longer/Cargo.lock"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cargoTestWorkspace(t)
			for _, f := range tc.files {
				cargoWrite(t, root, f, f)
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if !tc.detected {
				test.Equals(t, 0, len(tools))
				test.Equals(t, 0, len(dirs))
				test.Equals(t, "", key)
				return
			}
			test.Equals(t, []string{"cargo"}, tools)
			test.Equals(t, cargoPaths(filepath.Join(root, ".cache", "cargo")), dirs)
			test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte(tc.files[0]))), key)
		})
	}
}
func TestScenario_Cargo_Key(t *testing.T) {
	root := cargoTestWorkspace(t)
	cargoWrite(t, root, "Cargo.lock", "locked-v1")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	cargoWrite(t, root, "Cargo.toml", "unrelated")
	_, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, same)
	cargoWrite(t, root, "Cargo.lock", "locked-v2")
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, first != changed, "lock changes must invalidate")
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("locked-v2"))), changed)
}
func TestScenario_Cargo_MixedLegacy(t *testing.T) {
	root := cargoTestWorkspace(t)
	cargoWrite(t, root, "go.mod", "module example.test/app\n")
	legacyDirs, legacyTools, legacyKey, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"golang"}, legacyTools)
	test.Equals(t, []string{".go"}, legacyDirs)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("module example.test/app\n"))), legacyKey)
	cargoWrite(t, root, "Cargo.lock", "cargo lock")
	dirs, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"golang", "cargo"}, tools)
	test.Equals(t, append(legacyDirs, cargoPaths(filepath.Join(root, ".cache", "cargo"))...), dirs)
	test.Equals(t, legacyKey+fmt.Sprintf("%x", md5.Sum([]byte("cargo lock"))), key)
}
func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	root := cargoTestWorkspace(t)
	cargoWrite(t, root, "Cargo.lock", "locked")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
func TestScenario_Cargo_Allowlist(t *testing.T) {
	root := cargoTestWorkspace(t)
	t.Setenv("CARGO_HOME", ".cargo-home")
	cargoWrite(t, root, "Cargo.lock", "locked")
	for _, p := range []string{"credentials.toml", "credentials", "config.toml", "bin/cargo", "registry/src/source", "git/checkouts/source", "target/output", ".package-cache", ".global-cache"} {
		cargoWrite(t, root, filepath.Join(".cargo-home", p), "excluded")
	}
	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"cargo"}, tools)
	test.Equals(t, cargoPaths(filepath.Join(root, ".cargo-home")), dirs)
}
