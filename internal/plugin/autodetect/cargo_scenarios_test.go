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

func TestScenario_Cargo_Detection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    []string
		detected bool
	}{
		{"root", []string{"Cargo.lock"}, true},
		{"nested", []string{"app/Cargo.lock"}, true},
		{"deep", []string{"a/b/Cargo.lock"}, false},
		{"manifest", []string{"Cargo.toml"}, false},
		{"rootAndNested", []string{"Cargo.lock", "app/Cargo.lock"}, true},
		{"siblings", []string{"a/Cargo.lock", "b/Cargo.lock"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cargoWorkspace(t)
			for i, f := range tc.files {
				cargoWrite(t, f, fmt.Sprintf("lock-%d", i))
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if !tc.detected {
				test.Equals(t, 0, len(tools))
				test.Equals(t, "", key)
				return
			}
			test.Equals(t, []string{"cargo"}, tools)
			expected := "lock-0"
			if tc.name == "siblings" {
				expected = "lock-1"
			} // legacy equal-length tie chooses the later match
			test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte(expected))), key)
			test.Equals(t, []string{filepath.Join(root, ".cache", "cargo", "registry", "index"), filepath.Join(root, ".cache", "cargo", "registry", "cache"), filepath.Join(root, ".cache", "cargo", "git", "db")}, dirs)
		})
	}
}

func TestScenario_Cargo_Keys(t *testing.T) {
	cargoWorkspace(t)
	cargoWrite(t, "Cargo.lock", "first")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	cargoWrite(t, "Cargo.toml", "[package]\nname = 'unrelated'\n")
	_, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, same)
	cargoWrite(t, "Cargo.lock", "second")
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, first != changed, "lockfile must invalidate")
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("second"))), changed)
}

func TestScenario_Cargo_Mixed(t *testing.T) {
	root := cargoWorkspace(t)
	t.Setenv("GOCACHE", filepath.Join(root, "go-cache"))
	cargoWrite(t, "go.mod", "module example.com/test\n")
	dirs, tools, baseline, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	cargoWrite(t, "Cargo.lock", "cargo-lock")
	mixedDirs, mixedTools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, append(tools, "cargo"), mixedTools)
	test.Equals(t, dirs, mixedDirs[:len(dirs)])
	test.Equals(t, baseline+fmt.Sprintf("%x", md5.Sum([]byte("cargo-lock"))), key)
}

func TestScenario_Cargo_SkipPrepare(t *testing.T) {
	cargoWorkspace(t)
	cargoWrite(t, "Cargo.lock", "lock")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
