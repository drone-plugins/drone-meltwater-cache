package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func sbtWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	cwd, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HARNESS_WORKSPACE", "")
	t.Setenv("COURSIER_CACHE", "")
	return root
}

func Test_sbtPreparer_Paths(t *testing.T) {
	root := sbtWorkspace(t)
	for _, tc := range []struct{ name, env, home string }{
		{"default", "", filepath.Join(root, ".cache", "sbt")},
		{"relative", "custom/../downloads", filepath.Join(root, "downloads")},
		{"absolute", filepath.Join(root, "absolute"), filepath.Join(root, "absolute")},
		{"spaces", " sbt home ", filepath.Join(root, " sbt home ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COURSIER_CACHE", tc.env)
			for i := 0; i < 2; i++ {
				primary, err := newSbtPreparer().PrepareRepo(filepath.Join(root, "nested"))
				test.Ok(t, err)
				test.Equals(t, filepath.Join(tc.home, "https"), primary)
			}
			_, err := os.Stat(tc.home)
			test.Assert(t, os.IsNotExist(err), "preparer must not create cache/config")
		})
	}
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	t.Setenv("COURSIER_CACHE", "")
	p, err := newSbtPreparer().PrepareRepo(root)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".cache", "sbt", "https"), p)
}

func Test_sbtPreparer_NoMutation(t *testing.T) {
	root := sbtWorkspace(t)
	for _, content := range []string{"csrCacheDirectory := file(\"custom\")\n", "malformed Scala [", ""} {
		test.Ok(t, os.WriteFile("build.sbt", []byte(content), 0600))
		for i := 0; i < 2; i++ {
			_, err := newSbtPreparer().PrepareRepo(root)
			test.Ok(t, err)
		}
		got, err := os.ReadFile("build.sbt")
		test.Ok(t, err)
		test.Equals(t, content, string(got))
	}
	entries, err := os.ReadDir(root)
	test.Ok(t, err)
	test.Equals(t, 1, len(entries))
}
