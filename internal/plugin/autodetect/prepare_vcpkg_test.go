package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func vcpkgWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	cwd, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HARNESS_WORKSPACE", "")
	t.Setenv("VCPKG_DEFAULT_BINARY_CACHE", "")
	return root
}

func vcpkgWrite(t *testing.T, path, content string) {
	t.Helper()
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(content), 0644))
}

func Test_vcpkgPreparer_Paths(t *testing.T) {
	root := vcpkgWorkspace(t)
	for _, tc := range []struct{ name, env, want string }{
		{"default", "", filepath.Join(root, ".cache", "vcpkg")},
		{"relative", "custom/../vcpkg-bin", filepath.Join(root, "vcpkg-bin")},
		{"absolute", filepath.Join(root, "abs-vcpkg"), filepath.Join(root, "abs-vcpkg")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VCPKG_DEFAULT_BINARY_CACHE", tc.env)
			got, err := newVcpkgPreparer().PrepareRepo(filepath.Join(root, "sub"))
			test.Ok(t, err)
			test.Equals(t, tc.want, got)
			info, err := os.Stat(tc.want)
			test.Ok(t, err)
			test.Assert(t, info.IsDir(), "vcpkg binary cache directory must exist so cold builds succeed")
		})
	}
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	p, err := newVcpkgPreparer().PrepareRepo(root)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".cache", "vcpkg"), p)
}
