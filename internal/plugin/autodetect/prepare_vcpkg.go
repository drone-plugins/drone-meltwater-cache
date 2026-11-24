package autodetect

import (
	"os"
	"path/filepath"
)

type vcpkgPreparer struct{}

func newVcpkgPreparer() *vcpkgPreparer { return &vcpkgPreparer{} }

// vcpkgBinaryCache follows cwd-relative VCPKG_DEFAULT_BINARY_CACHE semantics,
// even for nested detection signals. The fallback matches Harness's workspace
// injection; build steps must receive VCPKG_DEFAULT_BINARY_CACHE so vcpkg
// stores and reads binary archives in the shared workspace.
func vcpkgBinaryCache(dir string) (string, error) {
	if configured := os.Getenv("VCPKG_DEFAULT_BINARY_CACHE"); configured != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return resolveRepoPath(cwd, configured)
	}
	return resolveRepoPath(workspaceRoot(dir), filepath.Join(".cache", "vcpkg"))
}

func (*vcpkgPreparer) PrepareRepo(dir string) (string, error) {
	cacheDir, err := vcpkgBinaryCache(dir)
	if err != nil {
		return "", err
	}
	// vcpkg fails on cold runs if VCPKG_DEFAULT_BINARY_CACHE does not exist yet.
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	return cacheDir, nil
}
