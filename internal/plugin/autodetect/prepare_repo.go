package autodetect

type RepoPreparer interface {
	// PrepareRepo change local files to a state where cache intelligence options can be performed
	PrepareRepo(dir string) (string, error)
}

// MultiRepoPreparer is implemented by preparers that need more than one cache
// directory (e.g. CocoaPods: the workspace-local Pods/ dir and the shared
// download cache under the user's home directory).
type MultiRepoPreparer interface {
	PrepareRepoMulti(dir string) ([]string, error)
}
