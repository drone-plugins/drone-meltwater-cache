package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func cargoWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	cwd, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HARNESS_WORKSPACE", "")
	t.Setenv("CARGO_HOME", "")
	return root
}

func Test_cargoPreparer_Paths(t *testing.T) {
	root := cargoWorkspace(t)
	for _, tc := range []struct{ name, env, home string }{
		{"default", "", filepath.Join(root, ".cache", "cargo")},
		{"relative", "custom/../downloads", filepath.Join(root, "downloads")},
		{"absolute", filepath.Join(root, "absolute"), filepath.Join(root, "absolute")},
		{"spaces", " cargo home ", filepath.Join(root, " cargo home ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARGO_HOME", tc.env)
			for i := 0; i < 2; i++ {
				primary, err := newCargoPreparer().PrepareRepo(filepath.Join(root, "nested"))
				test.Ok(t, err)
				test.Equals(t, filepath.Join(tc.home, "registry", "index"), primary)
				extra, err := cargoCacheDirs(root)
				test.Ok(t, err)
				test.Equals(t, []string{filepath.Join(tc.home, "registry", "cache"), filepath.Join(tc.home, "git", "db")}, extra)
			}
			_, err := os.Stat(tc.home)
			test.Assert(t, os.IsNotExist(err), "preparer must not create cache/config")
		})
	}
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	t.Setenv("CARGO_HOME", "")
	p, err := newCargoPreparer().PrepareRepo(root)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".cache", "cargo", "registry", "index"), p)
}

func Test_cargoPreparer_ConfigUntouched(t *testing.T) {
	root := cargoWorkspace(t)
	test.Ok(t, os.MkdirAll(".cargo", 0755))
	for _, content := range []string{"[net]\noffline = true\n", "this is not TOML [", ""} {
		test.Ok(t, os.WriteFile(".cargo/config.toml", []byte(content), 0600))
		_, err := newCargoPreparer().PrepareRepo(root)
		test.Ok(t, err)
		got, err := os.ReadFile(".cargo/config.toml")
		test.Ok(t, err)
		test.Equals(t, content, string(got))
	}
}
