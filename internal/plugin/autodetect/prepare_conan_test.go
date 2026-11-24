package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func conanWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	cwd, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HARNESS_WORKSPACE", "")
	t.Setenv("CONAN_HOME", "")
	return root
}

func conanWrite(t *testing.T, path, content string) {
	t.Helper()
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(content), 0644))
}

func Test_conanPreparer_Paths(t *testing.T) {
	root := conanWorkspace(t)
	for _, tc := range []struct{ name, env, want string }{
		{"default", "", filepath.Join(root, ".conan2", "p")},
		{"relative", "custom/../conan-home", filepath.Join(root, "conan-home", "p")},
		{"absolute", filepath.Join(root, "abs-conan"), filepath.Join(root, "abs-conan", "p")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CONAN_HOME", tc.env)
			got, err := newConanPreparer().PrepareRepo(filepath.Join(root, "sub"))
			test.Ok(t, err)
			test.Equals(t, tc.want, got)
			_, err = os.Stat(tc.want)
			test.Assert(t, os.IsNotExist(err), "detection must not create the cache")
		})
	}
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	p, err := newConanPreparer().PrepareRepo(root)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".conan2", "p"), p)
}
