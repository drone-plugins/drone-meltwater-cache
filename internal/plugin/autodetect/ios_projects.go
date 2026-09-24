package autodetect

import (
	"io/fs"
	"path/filepath"
	"sort"
)

// generatedIOSDirs are build outputs and dependency trees. Manifests inside
// them are produced by the tools, not the project's own declarations.
var generatedIOSDirs = map[string]struct{}{
	".git":         {},
	".build":       {},
	"DerivedData":  {},
	"Pods":         {},
	"node_modules": {},
	"vendor":       {},
}

func isPerProjectIOSTool(tool string) bool {
	switch tool {
	case "cocoapods", "spm", "fastlane":
		return true
	default:
		return false
	}
}

// appendPerProjectIOS caches every matching project for one iOS tool. The
// first mapping row for that tool performs the whole walk; later rows are
// skipped once the tool is recorded.
func appendPerProjectIOS(tool buildToolInfo, skipPrepare bool, directories *[]string, tools *[]string, hashes *string) error {
	manifests, err := collectIOSManifests(tool.tool)
	if err != nil || len(manifests) == 0 || skipPrepare {
		return err
	}

	hash, _, err := calculateMd5FromAllFilesPerProject(manifests)
	if err != nil {
		return err
	}

	switch tool.tool {
	case "cocoapods":
		for _, manifest := range manifests {
			dir, err := filepath.Abs(filepath.Dir(manifest))
			if err != nil {
				return err
			}

			cacheDir, err := tool.preparer.PrepareRepo(dir)
			if err != nil {
				return err
			}

			*directories = appendIfMissing(*directories, cacheDir)
		}

		if tool.additionalCacheDirs != nil {
			extra, err := tool.additionalCacheDirs("")
			if err != nil {
				return err
			}

			for _, dir := range extra {
				if dir != "" {
					*directories = appendIfMissing(*directories, dir)
				}
			}
		}
	case "fastlane":
		for _, manifest := range manifests {
			dir, err := filepath.Abs(filepath.Dir(manifest))
			if err != nil {
				return err
			}

			cacheDir, err := tool.preparer.PrepareRepo(dir)
			if err != nil {
				return err
			}

			*directories = appendIfMissing(*directories, cacheDir)
		}
	case "spm":
		for _, manifest := range manifests {
			cacheDirs, err := spmCacheDirsForManifest(manifest)
			if err != nil {
				return err
			}

			for _, dir := range cacheDirs {
				if dir != "" {
					*directories = appendIfMissing(*directories, dir)
				}
			}
		}
	}

	*tools = appendIfMissing(*tools, tool.tool)
	*hashes += hash

	return nil
}

func collectIOSManifests(tool string) ([]string, error) {
	files, err := walkRepoFiles()
	if err != nil {
		return nil, err
	}

	switch tool {
	case "cocoapods":
		return manifestsByDir(files, "Podfile.lock", "Podfile", nil), nil
	case "fastlane":
		return manifestsByDir(files, "Gemfile.lock", "Gemfile", iosProjectMarkers), nil
	case "spm":
		return spmManifests(files), nil
	default:
		return nil, nil
	}
}

// manifestsByDir keeps the lockfile in a directory when both the lockfile and
// the manifest are present, and otherwise keeps the manifest. Sibling
// directories stay independent.
func manifestsByDir(files []string, lockName, manifestName string, requires []string) []string {
	type pair struct {
		lock     string
		manifest string
	}

	byDir := map[string]*pair{}
	var dirs []string
	for _, file := range files {
		name := filepath.Base(file)
		if name != lockName && name != manifestName {
			continue
		}

		if requires != nil && !dirSatisfiesRequires(file, requires) {
			continue
		}

		dir := filepath.Dir(file)
		entry, ok := byDir[dir]
		if !ok {
			entry = &pair{}
			byDir[dir] = entry
			dirs = append(dirs, dir)
		}

		if name == lockName {
			entry.lock = file
		} else {
			entry.manifest = file
		}
	}

	sort.Strings(dirs)

	var manifests []string
	for _, dir := range dirs {
		entry := byDir[dir]
		if entry.lock != "" {
			manifests = append(manifests, entry.lock)
			continue
		}

		if entry.manifest != "" {
			manifests = append(manifests, entry.manifest)
		}
	}

	return manifests
}

func spmManifests(files []string) []string {
	type pair struct {
		resolved string
		manifest string
	}

	byDir := map[string]*pair{}
	var dirs []string
	var xcode []string
	for _, file := range files {
		name := filepath.Base(file)
		if name != "Package.resolved" && name != "Package.swift" {
			continue
		}

		if _, xcodeIntegrated := classifySPMManifest(file); xcodeIntegrated {
			xcode = append(xcode, file)
			continue
		}

		dir := filepath.Dir(file)
		entry, ok := byDir[dir]
		if !ok {
			entry = &pair{}
			byDir[dir] = entry
			dirs = append(dirs, dir)
		}

		if name == "Package.resolved" {
			entry.resolved = file
		} else {
			entry.manifest = file
		}
	}

	sort.Strings(dirs)
	sort.Strings(xcode)

	var manifests []string
	for _, dir := range dirs {
		entry := byDir[dir]
		if entry.resolved != "" {
			manifests = append(manifests, entry.resolved)
			continue
		}

		if entry.manifest != "" {
			manifests = append(manifests, entry.manifest)
		}
	}

	return append(manifests, xcode...)
}

func walkRepoFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if path != "." {
				if _, skip := generatedIOSDirs[entry.Name()]; skip {
					return filepath.SkipDir
				}
			}

			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}
