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
	t.Setenv("HARNESS_WORKSPACE", root)
	return root
}

func cargoExpectedPaths(home string) []string {
	return []string{filepath.Join(home, "registry", "cache"), filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}
}

func Test_cargoPreparer_Paths(t *testing.T) {
	root := cargoTestWorkspace(t)
	for _, tc := range []struct{ name, env, home string }{
		{"default", "", filepath.Join(root, ".cache", "cargo")},
		{"relative", "custom/../cargo", filepath.Join(root, "cargo")},
		{"absolute", filepath.Join(root, "elsewhere"), filepath.Join(root, "elsewhere")},
		{"whitespace", " cargo home ", filepath.Join(root, " cargo home ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARGO_HOME", tc.env)
			primary, err := newCargoPreparer().PrepareRepo(filepath.Join(root, "nested"))
			test.Ok(t, err)
			extra, err := cargoCacheDirs(filepath.Join(root, "nested"))
			test.Ok(t, err)
			test.Equals(t, cargoExpectedPaths(tc.home), append([]string{primary}, extra...))
		})
	}
	t.Setenv("HARNESS_WORKSPACE", "")
	got, err := newCargoPreparer().PrepareRepo(filepath.Join(root, "nested"))
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, ".cache", "cargo", "registry", "cache"), got)
}

func Test_cargoPreparer_NoMutation(t *testing.T) {
	root := cargoTestWorkspace(t)
	config := filepath.Join(root, ".cargo", "config.toml")
	test.Ok(t, os.MkdirAll(filepath.Dir(config), 0755))
	content := []byte("# user-owned malformed config is not interpreted by cache plugin\n[broken\n")
	test.Ok(t, os.WriteFile(config, content, 0600))
	for i := 0; i < 2; i++ {
		got, err := newCargoPreparer().PrepareRepo(root)
		test.Ok(t, err)
		test.Equals(t, filepath.Join(root, ".cache", "cargo", "registry", "cache"), got)
	}
	got, err := os.ReadFile(config)
	test.Ok(t, err)
	test.Equals(t, content, got)
	_, err = os.Stat(filepath.Join(root, ".cache"))
	test.Assert(t, os.IsNotExist(err), "preparer must not create paths")
	info, err := os.Stat(config)
	test.Ok(t, err)
	test.Equals(t, os.FileMode(0600), info.Mode().Perm())
}
