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
	old, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { test.Ok(t, os.Chdir(old)) })
	t.Setenv("CARGO_HOME", "")
	t.Setenv("HARNESS_WORKSPACE", "")
	return root
}

func cargoExpectedDirs(home string) []string {
	return []string{filepath.Join(home, "registry", "cache"), filepath.Join(home, "registry", "index"), filepath.Join(home, "git", "db")}
}

func Test_cargoPreparer_Paths(t *testing.T) {
	root := cargoTestWorkspace(t)
	project := filepath.Join(root, "nested")
	test.Ok(t, os.Mkdir(project, 0755))
	for _, tc := range []struct{ name, env, workspace, home string }{
		{"default", "", "", filepath.Join(root, ".cache", "cargo")},
		{"relative", "relative-home", "", filepath.Join(root, "relative-home")},
		{"normalized", filepath.Join("a", "..", "b"), "", filepath.Join(root, "b")},
		{"absolute", filepath.Join(root, "absolute"), "", filepath.Join(root, "absolute")},
		{"spaces", " home ", "", filepath.Join(root, " home ")},
		{"workspace", "", project, filepath.Join(project, ".cache", "cargo")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARGO_HOME", tc.env)
			t.Setenv("HARNESS_WORKSPACE", tc.workspace)
			primary, err := newCargoPreparer().PrepareRepo(project)
			test.Ok(t, err)
			extra, err := cargoCacheDirs(project)
			test.Ok(t, err)
			test.Equals(t, cargoExpectedDirs(tc.home), append([]string{primary}, extra...))
		})
	}
}

func Test_cargoPreparer_ConfigUntouched(t *testing.T) {
	root := cargoTestWorkspace(t)
	config := filepath.Join(root, ".cargo", "config.toml")
	test.Ok(t, os.MkdirAll(filepath.Dir(config), 0755))
	content := []byte("invalid TOML [\n# keep all original bytes\n")
	test.Ok(t, os.WriteFile(config, content, 0600))
	home := filepath.Join(root, "home")
	t.Setenv("CARGO_HOME", home)
	test.Ok(t, os.MkdirAll(home, 0755))
	for _, name := range []string{"config.toml", "credentials.toml", "credentials"} {
		test.Ok(t, os.WriteFile(filepath.Join(home, name), content, 0600))
	}
	for i := 0; i < 2; i++ {
		_, err := newCargoPreparer().PrepareRepo(root)
		test.Ok(t, err)
		_, err = cargoCacheDirs(root)
		test.Ok(t, err)
	}
	for _, path := range []string{config, filepath.Join(home, "config.toml"), filepath.Join(home, "credentials.toml"), filepath.Join(home, "credentials")} {
		got, err := os.ReadFile(path)
		test.Ok(t, err)
		test.Equals(t, content, got)
		info, err := os.Stat(path)
		test.Ok(t, err)
		test.Equals(t, os.FileMode(0600), info.Mode().Perm())
	}
	entries, err := os.ReadDir(home)
	test.Ok(t, err)
	test.Equals(t, 3, len(entries))
}
