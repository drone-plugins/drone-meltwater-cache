package autodetect

import (
	"os"
	"path/filepath"
)

type ccachePreparer struct{}

func newCcachePreparer() *ccachePreparer { return &ccachePreparer{} }

// ccacheRoot follows cwd-relative CCACHE_DIR semantics, even for nested
// detection signals. The fallback matches Harness's workspace injection;
// build steps must receive CCACHE_DIR and actually invoke ccache.
func ccacheRoot(dir string) (string, error) {
	if configured := os.Getenv("CCACHE_DIR"); configured != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, configured)
	}
	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "ccache"))
}

// Only archive ccache's hexadecimal data shards. In particular, the root
// ccache.conf may contain remote-storage credentials, and tmp is transient.
// Keep all sixteen paths stable across restore and save, even when absent.
func (*ccachePreparer) PrepareRepo(dir string) (string, error) {
	root, err := ccacheRoot(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "0"), nil
}

func ccacheAdditionalDirs(dir string) ([]string, error) {
	root, err := ccacheRoot(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, shard := range "123456789abcdef" {
		paths = append(paths, filepath.Join(root, string(shard)))
	}
	return paths, nil
}
