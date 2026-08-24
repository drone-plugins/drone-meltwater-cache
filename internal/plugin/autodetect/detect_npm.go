package autodetect

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	packageJSONFile  = "package.json"
	packageLockFile  = "package-lock.json"
	yarnLockFileName = "yarn.lock"
	pnpmLockFileName = "pnpm-lock.yaml"
	bunLockFileName  = "bun.lock"
	bunLockbFileName = "bun.lockb"
)

// npmProject is one directory that npm would install into.
type npmProject struct {
	dir               string
	file              string
	keyedFromLockfile bool
}

func detectNpmProjects(skipPrepare bool) (string, []string, error) {
	if skipPrepare {
		return "", nil, nil
	}

	projects, err := findNpmProjects()
	if err != nil {
		return "", nil, err
	}

	if len(projects) == 0 {
		return "", nil, nil
	}

	files := make([]string, 0, len(projects))
	for _, p := range projects {
		files = append(files, p.file)
	}

	hash, _, err := calculateMd5FromAllFilesPerProject(files)
	if err != nil {
		return "", nil, err
	}

	var dirs []string
	for _, p := range projects {
		absDir, err := filepath.Abs(p.dir)
		if err != nil {
			return "", nil, err
		}

		prepared, err := npmCacheDirs(absDir, p.keyedFromLockfile)
		if err != nil {
			return "", nil, err
		}
		for _, dir := range prepared {
			dirs = appendIfMissing(dirs, dir)
		}
	}

	if len(dirs) == 0 {
		return "", nil, nil
	}

	return hash, dirs, nil
}

func findNpmProjects() ([]npmProject, error) {
	if p, ok := npmProjectAt("."); ok {
		return []npmProject{p}, nil
	}

	dirs := map[string]struct{}{}

	for _, glob := range []string{
		filepath.Join("*", packageJSONFile),
		filepath.Join("*", packageLockFile),
	} {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return nil, err
		}

		for _, m := range matches {
			dirs[filepath.Dir(m)] = struct{}{}
		}
	}

	var projects []npmProject

	for dir := range dirs {
		if p, ok := npmProjectAt(dir); ok {
			projects = append(projects, p)
		}
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].dir < projects[j].dir })

	return projects, nil
}

// DetectNpmPackageJSONSources identifies npm projects that Restore keyed from
// package.json. Save uses these identities to avoid adding node_modules after
// npm generates a lockfile.
func DetectNpmPackageJSONSources() ([]string, error) {
	projects, err := findNpmProjects()
	if err != nil {
		return nil, err
	}
	var sources []string
	for _, project := range projects {
		if project.keyedFromLockfile {
			continue
		}
		dir, err := filepath.Abs(project.dir)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(filepath.Clean(dir)))
		sources = append(sources, fmt.Sprintf("%x", sum[:]))
	}
	sort.Strings(sources)
	return sources, nil
}

// FilterPathsForPlan removes node_modules paths that were not cacheable when
// Restore selected its package.json-derived key.
func FilterPathsForPlan(paths, packageJSONSources []string) []string {
	if len(packageJSONSources) == 0 {
		return paths
	}
	excluded := make(map[string]struct{}, len(packageJSONSources))
	for _, source := range packageJSONSources {
		excluded[source] = struct{}{}
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if filepath.Base(path) == nodeModulesDirName {
			sum := sha256.Sum256([]byte(filepath.Clean(filepath.Dir(path))))
			if _, found := excluded[fmt.Sprintf("%x", sum[:])]; found {
				continue
			}
		}
		filtered = append(filtered, path)
	}
	return filtered
}

func npmProjectAt(dir string) (npmProject, bool) {
	lock := filepath.Join(dir, packageLockFile)
	pkg := filepath.Join(dir, packageJSONFile)

	hasNPMLock := fileExists(lock)
	if hasOtherJSLock(dir) && !hasNPMLock {
		return npmProject{}, false
	}

	switch {
	case hasNPMLock:
		return npmProject{
			dir:               dir,
			file:              lock,
			keyedFromLockfile: true,
		}, true
	case fileExists(pkg):
		return npmProject{
			dir:  dir,
			file: pkg,
		}, true
	}

	return npmProject{}, false
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
