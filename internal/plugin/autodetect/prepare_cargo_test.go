package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func isolateCargoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CARGO_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HARNESS_WORKSPACE", "")
}

func Test_cargoPreparer_Paths(t *testing.T) {
	for _, mode := range []string{"default", "relative", "absolute", "workspace-home"} {
		t.Run(mode, func(t *testing.T) {
			isolateCargoEnv(t)
			root := setupCargoScenario(t, nil)
			t.Setenv("HARNESS_WORKSPACE", root)
			expected := filepath.Join(root, ".cache", "cargo")
			switch mode {
			case "relative":
				t.Setenv("CARGO_HOME", "custom/../cargo-home")
				expected = filepath.Join(root, "cargo-home")
			case "absolute":
				expected = filepath.Join(t.TempDir(), "cargo")
				t.Setenv("CARGO_HOME", expected)
			case "workspace-home":
				t.Setenv("HOME", root)
				t.Setenv("USERPROFILE", root)
				expected = filepath.Join(root, ".cargo")
			}
			dir := filepath.Join(root, "nested")
			for i := 0; i < 2; i++ {
				got, err := newCargoPreparer().PrepareRepo(dir)
				test.Ok(t, err)
				test.Equals(t, filepath.Join(expected, "registry", "cache"), got)
				extra, err := cargoCacheDirs(dir)
				test.Ok(t, err)
				test.Equals(t, []string{filepath.Join(expected, "registry", "index"), filepath.Join(expected, "git", "db")}, extra)
			}
			entries, err := os.ReadDir(root)
			test.Ok(t, err)
			test.Equals(t, 0, len(entries)) // no generated configuration or directories
		})
	}
}

func Test_cargoPreparer_PreservesConfigAndExcludesSecrets(t *testing.T) {
	isolateCargoEnv(t)
	root := setupCargoScenario(t, map[string]string{
		"Cargo.lock":                             "lock",
		".cargo/config.toml":                     "[source.crates-io]\nreplace-with = 'vendor'\n", // read-only, even malformed TOML is not ours to parse
		".cache/cargo/credentials.toml":          "secret",
		".cache/cargo/bin/tool":                  "binary",
		".cache/cargo/registry/src/crate/lib.rs": "source",
		".cache/cargo/git/checkouts/repo/file":   "source",
		"target/binary":                          "output",
	})
	before, err := os.ReadFile(filepath.Join(root, ".cargo/config.toml"))
	test.Ok(t, err)
	dirs, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, cargoExpectedDirs(filepath.Join(root, ".cache", "cargo")), dirs)
	after, err := os.ReadFile(filepath.Join(root, ".cargo/config.toml"))
	test.Ok(t, err)
	test.Equals(t, before, after)
}
