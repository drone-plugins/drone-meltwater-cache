package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func cargoTestWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	cwd, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { test.Ok(t, os.Chdir(cwd)) })
	t.Setenv("CARGO_HOME", "")
	t.Setenv("HARNESS_WORKSPACE", "")
	return root
}

func Test_cargoPreparer_Paths(t *testing.T) {
	root := cargoTestWorkspace(t)
	for _, tc := range []struct{ name, env, home string }{
		{"default", "", filepath.Join(root, ".cache", "cargo")},
		{"relative", "downloads/../cargo", filepath.Join(root, "cargo")},
		{"absolute", filepath.Join(root, "external"), filepath.Join(root, "external")},
		{"spaces", "cargo home", filepath.Join(root, "cargo home")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARGO_HOME", tc.env)
			path, err := newCargoPreparer().PrepareRepo(filepath.Join(root, "nested"))
			test.Ok(t, err)
			test.Equals(t, filepath.Join(tc.home, "registry", "index"), path)
			dirs, err := cargoCacheDirs(root)
			test.Ok(t, err)
			test.Equals(t, []string{filepath.Join(tc.home, "registry", "cache"), filepath.Join(tc.home, "git", "db")}, dirs)
		})
	}
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	path, err := newCargoPreparer().PrepareRepo(root)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".cache", "cargo", "registry", "index"), path)
}

func Test_cargoPreparer_NoMutation(t *testing.T) {
	root := cargoTestWorkspace(t)
	for _, data := range []string{"", "not valid TOML [", "[registries.private]\nindex = 'https://example.test/index'\n"} {
		test.Ok(t, os.MkdirAll(filepath.Join(root, ".cargo"), 0755))
		cfg := filepath.Join(root, ".cargo", "config.toml")
		test.Ok(t, os.WriteFile(cfg, []byte(data), 0600))
		first, err := newCargoPreparer().PrepareRepo(root)
		test.Ok(t, err)
		second, err := newCargoPreparer().PrepareRepo(root)
		test.Ok(t, err)
		test.Equals(t, first, second)
		got, err := os.ReadFile(cfg)
		test.Ok(t, err)
		test.Equals(t, data, string(got))
		info, err := os.Stat(cfg)
		test.Ok(t, err)
		test.Equals(t, os.FileMode(0600), info.Mode().Perm())
		_, err = os.Stat(filepath.Join(root, ".cache"))
		test.Assert(t, os.IsNotExist(err), "must not create cache")
	}
}
