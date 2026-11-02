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

func cargoExpectedPaths(home string) []string {
	return []string{filepath.Join(home, "registry", "cache"), filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}
}

func Test_cargoPreparer_Paths(t *testing.T) {
	root := cargoTestWorkspace(t)
	for _, tc := range []struct{ name, env, ws, home string }{
		{"default", "", "", filepath.Join(root, ".cache", "cargo")},
		{"relative", "local/../downloads", "", filepath.Join(root, "downloads")},
		{"absolute", filepath.Join(root, "explicit"), "", filepath.Join(root, "explicit")},
		{"workspace", "", filepath.Join(root, "shared"), filepath.Join(root, "shared", ".cache", "cargo")},
		{"whitespace-is-literal", " cargo ", "", filepath.Join(root, " cargo ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARGO_HOME", tc.env)
			t.Setenv("HARNESS_WORKSPACE", tc.ws)
			p, err := newCargoPreparer().PrepareRepo(filepath.Join(root, "nested"))
			test.Ok(t, err)
			extra, err := cargoCacheDirs(filepath.Join(root, "nested"))
			test.Ok(t, err)
			test.Equals(t, cargoExpectedPaths(tc.home), append([]string{p}, extra...))
			_, err = os.Stat(tc.home)
			test.Assert(t, os.IsNotExist(err), "preparation must not create home")
		})
	}
}

func Test_cargoPreparer_PreservesConfig(t *testing.T) {
	root := cargoTestWorkspace(t)
	t.Setenv("CARGO_HOME", filepath.Join(root, "home"))
	files := map[string]string{".cargo/config.toml": "[broken", "home/config.toml": "[net]\noffline = true\n", "home/credentials.toml": "dummy sentinel"}
	for p, s := range files {
		test.Ok(t, os.MkdirAll(filepath.Dir(p), 0755))
		test.Ok(t, os.WriteFile(p, []byte(s), 0600))
	}
	for i := 0; i < 2; i++ {
		p, err := newCargoPreparer().PrepareRepo(root)
		test.Ok(t, err)
		extra, err := cargoCacheDirs(root)
		test.Ok(t, err)
		test.Equals(t, cargoExpectedPaths(filepath.Join(root, "home")), append([]string{p}, extra...))
	}
	for p, s := range files {
		b, err := os.ReadFile(p)
		test.Ok(t, err)
		test.Equals(t, s, string(b))
	}
}
