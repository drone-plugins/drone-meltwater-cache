package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type buildToolInfo struct {
	globToDetect  string
	tool          string
	preparer      interface{}
	usePerProject bool
	// alsoDetect lists additional patterns to try when globToDetect finds
	// nothing, in order. Needed because Go's filepath.Glob treats "**" as a
	// single path element rather than a recursive wildcard, so tools that bury
	// their manifest at a fixed but deeper location (Xcode's Package.resolved)
	// are invisible to the root + one-level probe.
	alsoDetect []string
	// requires, if non-empty, gates detection on at least one of these globs
	// also being present *in the same directory as the detected manifest*. Used
	// to scope Gemfile detection to iOS repos only (CI-23961 is iOS/Fastlane
	// specific, not general Ruby support) without affecting tools that have no
	// such restriction.
	requires []string
}

// probePatterns returns the glob patterns to try for a tool, in precedence
// order. Every pattern is also probed one directory deep, preserving the
// long-standing "fall back to **/<glob>" behaviour.
func probePatterns(info buildToolInfo) []string {
	globs := make([]string, 0, 1+len(info.alsoDetect))
	globs = append(globs, info.globToDetect)
	globs = append(globs, info.alsoDetect...)

	patterns := make([]string, 0, len(globs)*2)
	for _, glob := range globs {
		patterns = append(patterns, glob, filepath.Join("**", glob))
	}

	return patterns
}

// dirSatisfiesRequires reports whether a detected manifest sits alongside at
// least one of the required marker files.
//
// The check is deliberately scoped to the manifest's own directory rather than
// the whole repository: a repo-wide check lets an ios/Podfile switch on Fastlane
// detection for an unrelated backend Gemfile in a monorepo, and then rewrite
// that project's Bundler config.
func dirSatisfiesRequires(manifest string, requires []string) bool {
	if len(requires) == 0 {
		return true
	}

	base := filepath.Dir(manifest)

	for _, glob := range requires {
		if matches, _ := filepath.Glob(filepath.Join(base, glob)); len(matches) > 0 {
			return true
		}
	}

	return false
}

func matchesSatisfying(pattern string, requires []string) []string {
	matches, _ := filepath.Glob(pattern)

	if len(requires) == 0 {
		return matches
	}

	kept := make([]string, 0, len(matches))

	for _, match := range matches {
		if dirSatisfiesRequires(match, requires) {
			kept = append(kept, match)
		}
	}

	return kept
}

// hashFirstMatch tries each pattern in order and hashes the first one that
// yields matches satisfying requires.
func hashFirstMatch(patterns, requires []string) (string, string, error) {
	for _, pattern := range patterns {
		if matches := matchesSatisfying(pattern, requires); len(matches) > 0 {
			return calculateMd5FromFiles(matches)
		}
	}

	return "", "", nil
}

func hashFirstMatchPerProject(patterns, requires []string) (string, []string, error) {
	for _, pattern := range patterns {
		if matches := matchesSatisfying(pattern, requires); len(matches) > 0 {
			return calculateMd5FromAllFilesPerProject(matches)
		}
	}

	return "", nil, nil
}

// prepareRepo runs preparer against dir and returns every path it wants cached.
// Supports both single-path (RepoPreparer) and multi-path (MultiRepoPreparer)
// preparers so existing tools stay untouched while e.g. CocoaPods can report
// more than one directory.
func prepareRepo(preparer interface{}, dir string) ([]string, error) {
	if mp, ok := preparer.(MultiRepoPreparer); ok {
		return mp.PrepareRepoMulti(dir)
	}

	if p, ok := preparer.(RepoPreparer); ok {
		dirToCache, err := p.PrepareRepo(dir)
		if err != nil {
			return nil, err
		}

		return []string{dirToCache}, nil
	}

	return nil, fmt.Errorf("preparer %T implements neither RepoPreparer nor MultiRepoPreparer", preparer)
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
		// Podfile.lock is checked BEFORE Podfile for the same reason as above:
		// prefer keying off the lock file when both exist.
		{
			globToDetect: "Podfile.lock",
			tool:         "cocoapods",
			preparer:     newCocoapodsPreparer(),
		},
		{
			globToDetect: "Podfile",
			tool:         "cocoapods",
			preparer:     newCocoapodsPreparer(),
		},
		// Package.resolved is checked BEFORE Package.swift so that, when both
		// exist, the cache key is derived from the lock file rather than the
		// manifest. Mirrors the MODULE.bazel-before-WORKSPACE precedence above.
		//
		// A pure SwiftPM package keeps Package.resolved at the root, but an Xcode
		// app integrating SPM through the UI keeps it inside the project or
		// workspace bundle instead -- and such a project usually has no
		// Package.swift at all, so without these patterns SPM goes completely
		// undetected for the most common iOS layout.
		{
			globToDetect: "Package.resolved",
			alsoDetect: []string{
				filepath.Join("*.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved"),
				filepath.Join("*.xcodeproj", "project.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved"),
			},
			tool:     "spm",
			preparer: newSPMPreparer(),
		},
		{
			globToDetect: "Package.swift",
			tool:         "spm",
			preparer:     newSPMPreparer(),
		},
		// Gemfile detection is scoped to iOS repos only (CI-23961 is
		// iOS/Fastlane-specific, not general Ruby/Bundler support), gated on the
		// presence of one of the common iOS project markers. Gemfile.lock is
		// checked before Gemfile for the same lock-file-first reason as above.
		{
			globToDetect: "Gemfile.lock",
			tool:         "fastlane",
			preparer:     newFastlanePreparer(),
			requires:     []string{"Podfile", "Package.swift", "*.xcodeproj", "*.xcworkspace"},
		},
		{
			globToDetect: "Gemfile",
			tool:         "fastlane",
			preparer:     newFastlanePreparer(),
			requires:     []string{"Podfile", "Package.swift", "*.xcodeproj", "*.xcworkspace"},
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

		patterns := probePatterns(supportedTool)
		requires := supportedTool.requires

		if supportedTool.usePerProject {
			hash, dirs, err := hashFirstMatchPerProject(patterns, requires)
			if err != nil {
				return nil, nil, "", err
			}
			if hash != "" && !skipPrepare {
				for _, dir := range dirs {
					dirsToCache, err := prepareRepo(supportedTool.preparer, dir)
					if err != nil {
						return nil, nil, "", err
					}
					for _, dirToCache := range dirsToCache {
						directoriesToCache = appendIfMissing(directoriesToCache, dirToCache)
					}
				}
				buildToolsDetected = appendIfMissing(buildToolsDetected, supportedTool.tool)
				hashes += hash
			}
		} else {
			hash, dir, err := hashFirstMatch(patterns, requires)
			if err != nil {
				return nil, nil, "", err
			}
			if hash != "" && !skipPrepare {
				dirsToCache, err := prepareRepo(supportedTool.preparer, dir)
				if err != nil {
					return nil, nil, "", err
				}

				for _, dirToCache := range dirsToCache {
					directoriesToCache = appendIfMissing(directoriesToCache, dirToCache)
				}
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
