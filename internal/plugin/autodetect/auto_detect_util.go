package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type buildToolInfo struct {
	globToDetect string
	tool         string
	preparer     RepoPreparer
	// extsToDetect lets a tool detect multiple file extensions in a single pass
	// with a recursive walk rather than Go's non-recursive filepath.Glob. When
	// set, globToDetect is used only as a label and the walk in
	// findFilesByExtensions takes precedence.
	extsToDetect []string
}

// containsTool checks if a tool is already in the slice
func containsTool(slice []string, tool string) bool {
	for _, v := range slice {
		if v == tool {
			return true
		}
	}
	return false
}

func DetectDirectoriesToCache(skipPrepare bool) ([]string, []string, string, error) {
	var buildToolInfoMapping = []buildToolInfo{
		{
			globToDetect: "pom.xml",
			tool:         "maven",
			preparer:     newMavenPreparer(),
		},
		{
			globToDetect: "build.gradle.kts",
			tool:         "gradle",
			preparer:     newGradlePreparer(),
		},
		{
			globToDetect: "build.gradle",
			tool:         "gradle",
			preparer:     newGradlePreparer(),
		},
		// MODULE.bazel is checked BEFORE WORKSPACE because:
		// 1. In modern Bazel (6+), MODULE.bazel takes precedence
		// 2. We only want ONE Bazel preparer to run, not both
		{
			globToDetect: "MODULE.bazel",
			tool:         "bazel",
			preparer:     newBzlmodPreparer(),
		},
		{
			globToDetect: "WORKSPACE",
			tool:         "bazel",
			preparer:     newBazelPreparer(),
		},
		{
			globToDetect: "package.json",
			tool:         "node",
			preparer:     newNodePreparer(),
		},
		{
			globToDetect: "yarn.lock",
			tool:         "yarn",
			preparer:     newYarnPreparer(),
		},
		{
			globToDetect: "go.mod",
			tool:         "golang",
			preparer:     newGoPreparer(),
		},
		{
			globToDetect: "*.csproj",
			tool:         "dotnet",
			preparer:     newDotnetPreparer(),
			extsToDetect: []string{".csproj", ".vbproj", ".fsproj"},
		},
	}

	var directoriesToCache []string

	var buildToolsDetected []string

	var hashes string

	for _, supportedTool := range buildToolInfoMapping {
		// Skip if this tool type was already detected
		// This prevents running both bazelPreparer and bzlmodPreparer
		// when a project has both WORKSPACE and MODULE.bazel
		if containsTool(buildToolsDetected, supportedTool.tool) {
			continue
		}

		var (
			hash string
			dir  string
			err  error
		)
		if len(supportedTool.extsToDetect) > 0 {
			hash, dir, err = hashAllFilesByExtensions(".", supportedTool.extsToDetect)
		} else {
			hash, dir, err = hashIfFileExist(supportedTool.globToDetect)
			if err == nil && hash == "" {
				hash, dir, err = hashIfFileExist(filepath.Join("**", supportedTool.globToDetect))
			}
		}
		if err != nil {
			return nil, nil, "", err
		}
		if hash != "" && !skipPrepare {
			dirToCache, err := supportedTool.preparer.PrepareRepo(dir)
			if err != nil {
				return nil, nil, "", err
			}

			directoriesToCache = appendIfMissing(directoriesToCache, dirToCache)
			buildToolsDetected = appendIfMissing(buildToolsDetected, supportedTool.tool)
			hashes += hash
		}
	}

	return directoriesToCache, buildToolsDetected, hashes, nil
}

func appendIfMissing(slice []string, elem string) []string {
	for _, v := range slice {
		if v == elem {
			return slice
		}
	}
	return append(slice, elem)
}

func hashIfFileExist(glob string) (string, string, error) {
	matches, _ := filepath.Glob(glob)

	if len(matches) == 0 {
		return "", "", nil
	}

	return calculateMd5FromFiles(matches)
}

func calculateMd5FromFiles(fileList []string) (string, string, error) {
	rootMostFile := shortestPath(fileList)
	file, err := os.Open(rootMostFile)

	if err != nil {
		return "", "", err
	}

	dir, err := filepath.Abs(filepath.Dir(rootMostFile))

	if err != nil {
		return "", "", err
	}

	defer file.Close()

	hash := md5.New() // #nosec
	_, err = io.Copy(hash, file)

	if err != nil {
		return "", "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), dir, nil
}

// hashAllFilesByExtensions walks root recursively and hashes every file whose
// extension matches one of exts, producing a single stable digest so the cache
// key reflects every project file in a multi-project repo (e.g. a .NET
// solution with several .csproj files).
//
// Returns (hex digest, common parent dir, error). The returned dir is root's
// absolute path so a single nuget.config / .nuget/packages location covers
// all detected projects.
func hashAllFilesByExtensions(root string, exts []string) (string, string, error) {
	matches, err := findFilesByExtensions(root, exts)
	if err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", "", nil
	}

	// Sort so the digest is independent of walk order across filesystems.
	sort.Strings(matches)

	hash := md5.New() // #nosec
	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			return "", "", err
		}
		if _, err := io.Copy(hash, f); err != nil {
			f.Close()
			return "", "", err
		}
		f.Close()
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), absRoot, nil
}

// findFilesByExtensions walks root and returns every regular file whose
// extension matches one of exts. Well-known build-output, VCS and dependency
// directories are skipped so vendored/generated project files do not leak into
// the cache key.
func findFilesByExtensions(root string, exts []string) ([]string, error) {
	skipDirs := map[string]bool{
		".git":         true,
		".hg":          true,
		".svn":         true,
		"node_modules": true,
		"bin":          true,
		"obj":          true,
		".vs":          true,
		".nuget":       true,
	}
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}

	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if path != root && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if extSet[strings.ToLower(filepath.Ext(info.Name()))] {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func shortestPath(input []string) (shortest string) {
	size := len(input[0])
	for _, v := range input {
		if len(v) <= size {
			shortest = v
			size = len(v)
		}
	}

	return
}
