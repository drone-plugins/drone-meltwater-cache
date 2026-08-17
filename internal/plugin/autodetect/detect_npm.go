package autodetect

import (
	"os"
	"path/filepath"
)

const (
	packageJSONFile   = "package.json"
	packageLockFile   = "package-lock.json"
	npmShrinkwrapFile = "npm-shrinkwrap.json"
	yarnLockFileName  = "yarn.lock"
	pnpmLockFileName  = "pnpm-lock.yaml"
	bunLockFileName   = "bun.lock"
	bunLockbFileName  = "bun.lockb"
)

// detectNpmProjects hashes npm projects at the same glob depth as lockfile
// detection: the repo root if it is npm, otherwise each one-level child.
// Yarn/pnpm/bun dirs are skipped unless an npm lockfile is also present.
// When git is available, only tracked lockfiles and package.json are hashed
// so a lockfile created by npm install does not change the Save key.
func detectNpmProjects() (string, []string, error) {
	tracked, useGit := gitTrackedNPMFiles()

	if f := npmFileToHash(".", tracked, useGit); f != "" {
		return calculateMd5FromAllFilesPerProject([]string{f})
	}

	dirs := map[string]struct{}{}
	for _, glob := range []string{
		filepath.Join("*", packageJSONFile),
		filepath.Join("*", packageLockFile),
		filepath.Join("*", npmShrinkwrapFile),
	} {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return "", nil, err
		}
		for _, m := range matches {
			dirs[filepath.Dir(m)] = struct{}{}
		}
	}

	var files []string
	for dir := range dirs {
		if f := npmFileToHash(dir, tracked, useGit); f != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return "", nil, nil
	}
	return calculateMd5FromAllFilesPerProject(files)
}

func npmFileToHash(dir string, tracked map[string]struct{}, useGit bool) string {
	lock := filepath.Join(dir, packageLockFile)
	shrink := filepath.Join(dir, npmShrinkwrapFile)
	pkg := filepath.Join(dir, packageJSONFile)
	hasNPMLock := npmFingerprintExists(lock, tracked, useGit) || npmFingerprintExists(shrink, tracked, useGit)
	if hasOtherJSLock(dir) && !hasNPMLock {
		return ""
	}
	if npmFingerprintExists(lock, tracked, useGit) {
		return lock
	}
	if npmFingerprintExists(shrink, tracked, useGit) {
		return shrink
	}
	if npmFingerprintExists(pkg, tracked, useGit) {
		return pkg
	}
	return ""
}

func hasOtherJSLock(dir string) bool {
	for _, name := range []string{yarnLockFileName, pnpmLockFileName, bunLockFileName, bunLockbFileName} {
		if fileExists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
