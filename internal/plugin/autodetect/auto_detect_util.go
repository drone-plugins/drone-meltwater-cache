package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type buildToolInfo struct {
	globToDetect  string
	tool          string
	preparer      RepoPreparer
	usePerProject bool
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
			globToDetect:  "*.csproj",
			tool:          "dotnet",
			preparer:      newDotnetPreparer(),
			usePerProject: true,
		},
		{
			globToDetect:  "*.vbproj",
			tool:          "dotnet",
			preparer:      newDotnetPreparer(),
			usePerProject: true,
		},
		{
			globToDetect:  "*.fsproj",
			tool:          "dotnet",
			preparer:      newDotnetPreparer(),
			usePerProject: true,
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

		if supportedTool.usePerProject {
			hash, dirs, err := hashAllFilesPerProjectIfExist(supportedTool.globToDetect)
			if err != nil {
				return nil, nil, "", err
			}
			if hash == "" {
				hash, dirs, err = hashAllFilesPerProjectIfExist(filepath.Join("**", supportedTool.globToDetect))
				if err != nil {
					return nil, nil, "", err
				}
			}
			if hash != "" && !skipPrepare {
				for _, dir := range dirs {
					dirToCache, err := supportedTool.preparer.PrepareRepo(dir)
					if err != nil {
						return nil, nil, "", err
					}
					directoriesToCache = appendIfMissing(directoriesToCache, dirToCache)
				}
				buildToolsDetected = appendIfMissing(buildToolsDetected, supportedTool.tool)
				hashes += hash
			}
		} else {
			hash, dir, err := hashIfFileExist(supportedTool.globToDetect)
			if err != nil {
				return nil, nil, "", err
			}
			if hash == "" {
				hash, dir, err = hashIfFileExist(filepath.Join("**", supportedTool.globToDetect))
				if err != nil {
					return nil, nil, "", err
				}
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

func hashAllFilesPerProjectIfExist(glob string) (string, []string, error) {
	matches, _ := filepath.Glob(glob)

	if len(matches) == 0 {
		return "", nil, nil
	}

	return calculateMd5FromAllFilesPerProject(matches)
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

// calculateMd5FromAllFilesPerProject hashes all files in the list and returns
// a deduplicated slice of their absolute parent directories. Used for .NET
// projects so each project directory gets its own nuget.config and cache entry.
//
// Files are sorted before hashing to ensure a stable cache key regardless
// of the order returned by filepath.Glob (which is filesystem-dependent).
func calculateMd5FromAllFilesPerProject(fileList []string) (string, []string, error) {
	if len(fileList) == 0 {
		return "", nil, nil
	}

	// Work on a sorted copy so hash is independent of input/Glob order.
	sorted := make([]string, len(fileList))
	copy(sorted, fileList)
	sort.Strings(sorted)

	hash := md5.New() // #nosec
	var dirs []string
	for _, filePath := range sorted {
		file, err := os.Open(filePath)
		if err != nil {
			return "", nil, err
		}
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return "", nil, err
		}
		absDir, err := filepath.Abs(filepath.Dir(filePath))
		if err != nil {
			return "", nil, err
		}
		dirs = appendIfMissing(dirs, absDir)
	}

	// Return directories in sorted order for stable, predictable output.
	sort.Strings(dirs)

	return hex.EncodeToString(hash.Sum(nil)), dirs, nil
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
