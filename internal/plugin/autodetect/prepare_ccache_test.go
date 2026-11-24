package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func ccacheWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	cwd, err := os.Getwd()
	test.Ok(t, err)
	test.Ok(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HARNESS_WORKSPACE", "")
	t.Setenv("CCACHE_DIR", "")
	return root
}

func ccacheWrite(t *testing.T, path, content string) {
	t.Helper()
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	test.Ok(t, os.WriteFile(path, []byte(content), 0644))
}

func ccacheShards(root string) []string {
	var paths []string
	for _, shard := range "0123456789abcdef" {
		paths = append(paths, filepath.Join(root, string(shard)))
	}
	return paths
}

func Test_ccachePreparer_Paths(t *testing.T) {
	root := ccacheWorkspace(t)
	for _, tc := range []struct{ name, env, want string }{
		{"default", "", filepath.Join(root, ".cache", "ccache")},
		{"relative", "custom/../compiler-cache", filepath.Join(root, "compiler-cache")},
		{"absolute", filepath.Join(root, "absolute"), filepath.Join(root, "absolute")},
		{"spaces", " cache data ", filepath.Join(root, " cache data ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CCACHE_DIR", tc.env)
			for i := 0; i < 2; i++ {
				primary, err := newCcachePreparer().PrepareRepo(filepath.Join(root, "cmake"))
				test.Ok(t, err)
				extra, err := ccacheAdditionalDirs(filepath.Join(root, "cmake"))
				test.Ok(t, err)
				test.Equals(t, ccacheShards(tc.want), append([]string{primary}, extra...))
			}
			_, err := os.Stat(tc.want)
			test.Assert(t, os.IsNotExist(err), "detection must not create the cache")
		})
	}
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	p, err := newCcachePreparer().PrepareRepo(root)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".cache", "ccache", "0"), p)
}

func Test_ccachePreparer_ConfigUntouched(t *testing.T) {
	root := ccacheWorkspace(t)
	cache := filepath.Join(root, "cache")
	t.Setenv("CCACHE_DIR", cache)
	t.Setenv("CCACHE_CONFIGPATH", filepath.Join(root, "native.conf"))
	// Malformed/unrelated native config is the real tool's responsibility. Never
	// read or rewrite config, and never archive a root that might hold secrets.
	for _, path := range []string{"ccache.conf", "native.conf", "cache/ccache.conf", "cache/tmp/transient"} {
		ccacheWrite(t, path, "unparsed config\r\nremote_storage = https://example.invalid\r\n")
	}
	before, err := os.ReadFile("cache/ccache.conf")
	test.Ok(t, err)
	primary, err := newCcachePreparer().PrepareRepo(root)
	test.Ok(t, err)
	extra, err := ccacheAdditionalDirs(root)
	test.Ok(t, err)
	paths := append([]string{primary}, extra...)
	test.Equals(t, ccacheShards(cache), paths)
	for _, path := range paths {
		test.Assert(t, path != cache && filepath.Base(path) != "tmp" && filepath.Base(path) != "ccache.conf", "only data shards are allowed")
	}
	after, err := os.ReadFile("cache/ccache.conf")
	test.Ok(t, err)
	test.Equals(t, string(before), string(after))
	// Populating a shard must not change which paths are selected.
	ccacheWrite(t, "cache/a/b/result", "cache-data")
	again, err := ccacheAdditionalDirs(root)
	test.Ok(t, err)
	test.Equals(t, extra, again)
}
