package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

type buildToolInfo struct {
	globToDetect string
	tool         string
	preparer     RepoPreparer
}

func DetectDirectoriesToCache(skipPrepare bool) ([]string, []string, string, error) {
	var buildToolInfoMapping = []buildToolInfo{
		{
			globToDetect: "pom.xml",
			tool:         "maven",
			preparer:     newMavenPreparer(),
		},
		{
			globToDetect: "build.gradle",
			tool:         "gradle",
			preparer:     newGradlePreparer(),
		},
		{
			globToDetect: "WORKSPACE",
			tool:         "bazel",
			preparer:     newBazelPreparer(),
		},
		{
			globToDetect: "MODULE.bazel",
			tool:         "bazel",
			preparer:     newBzlmodPreparer(),
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
		},
		{
			globToDetect: "*.vbproj",
			tool:         "dotnet",
			preparer:     newDotnetPreparer(),
		},
		{
			globToDetect: "*.fsproj",
			tool:         "dotnet",
			preparer:     newDotnetPreparer(),
		},
	}

	var directoriesToCache []string

	var buildToolsDetected []string

	var hashes string

	for _, supportedTool := range buildToolInfoMapping {
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

	if err != nil {
		return "", "", err
	}

	hash := md5.New() // #nosec
	_, err = io.Copy(hash, file)

	if err != nil {
		return "", "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), dir, nil
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
