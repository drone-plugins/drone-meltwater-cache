package autodetect

import (
	"os"
	"path/filepath"
)

type sbtPreparer struct{}

func newSbtPreparer() *sbtPreparer { return &sbtPreparer{} }

// PrepareRepo selects Coursier's HTTPS downloads, not sbt boot, Ivy config or
// build outputs. Build steps must use the same COURSIER_CACHE; the default
// matches Harness's workspace injection. Explicit sbt cache settings must agree
// with that variable. Never execute arbitrary Scala build definitions here.
func (*sbtPreparer) PrepareRepo(dir string) (string, error) {
	root := workspaceRoot(dir)
	cache := os.Getenv("COURSIER_CACHE")
	if cache == "" {
		cache = filepath.Join(".cache", "sbt")
	} else if !filepath.IsAbs(cache) {
		// Coursier resolves relative env values from its process cwd, even
		// when the detected lockfile belongs to a nested project.
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = cwd
	}
	home, err := resolveRepoPath(root, cache)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "https"), nil
}
