package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestScenario_AllJSLockfilesPresent_ClassicYarn(t *testing.T) {
	isolateNpmEnv(t)
	root := t.TempDir()
	t.Chdir(root)

	// Create all JS package manager lockfiles and manifests in the same root (Yarn 1)
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"all-js-test"}`), 0644))
	test.Ok(t, os.WriteFile("package-lock.json", []byte(`{"name":"all-js-test","lockfileVersion":3}`), 0644))
	test.Ok(t, os.WriteFile("yarn.lock", []byte("# yarn lockfile v1\n"), 0644))
	test.Ok(t, os.WriteFile("pnpm-lock.yaml", []byte("lockfileVersion: '9.0'\n"), 0644))
	test.Ok(t, os.WriteFile("bun.lock", []byte("bun-text-lockfile"), 0644))
	test.Ok(t, os.WriteFile("bun.lockb", []byte("bun-binary-lockfile"), 0644))

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// All 4 tools are detected
	test.Equals(t, []string{"node", "yarn", "pnpm", "bun"}, buildToolsDetected)

	// Dirs to cache include node_modules, .yarn, .cache/pnpm, .cache/bun (deduplicated)
	expectedDirs := []string{
		filepath.Join(root, "node_modules"),
		filepath.Join(root, ".yarn"),
		filepath.Join(root, ".cache", "pnpm"),
		filepath.Join(root, ".cache", "bun"),
	}
	test.Equals(t, expectedDirs, directoriesToCache)

	// Hash contains: md5(package-lock.json) + md5(package.json)+md5(yarn.lock) + md5(package.json)+md5(pnpm-lock.yaml) + md5(package.json)+md5(bun.lock)
	// Total 7 md5 hashes = 7 * 32 = 224 chars. bun.lockb is excluded because bun.lock is present.
	test.Equals(t, 224, len(hashes))
}

func TestScenario_AllJSLockfilesPresent_YarnBerry(t *testing.T) {
	isolateNpmEnv(t)
	root := t.TempDir()
	t.Chdir(root)

	// Create all JS package manager lockfiles and manifests in the same root with Yarn Berry (.yarnrc.yml)
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"all-js-test","packageManager":"yarn@4.9.1"}`), 0644))
	test.Ok(t, os.WriteFile("package-lock.json", []byte(`{"name":"all-js-test","lockfileVersion":3}`), 0644))
	test.Ok(t, os.WriteFile("yarn.lock", []byte(berryLock), 0644))
	test.Ok(t, os.WriteFile(".yarnrc.yml", []byte("nodeLinker: node-modules\n"), 0644))
	test.Ok(t, os.WriteFile("pnpm-lock.yaml", []byte("lockfileVersion: '9.0'\n"), 0644))
	test.Ok(t, os.WriteFile("bun.lock", []byte("bun-text-lockfile"), 0644))

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"node", "yarn", "pnpm", "bun"}, buildToolsDetected)

	// Dirs for Yarn Berry: .yarn/cache, node_modules, .yarn/install-state.gz
	expectedDirs := []string{
		filepath.Join(root, "node_modules"),
		filepath.Join(root, ".yarn", "cache"),
		filepath.Join(root, ".yarn", "install-state.gz"),
		filepath.Join(root, ".cache", "pnpm"),
		filepath.Join(root, ".cache", "bun"),
	}
	test.Equals(t, expectedDirs, directoriesToCache)
	test.Equals(t, 224, len(hashes))
}

func TestScenario_PackageJSONWithOtherLockfiles_NoPackageLock(t *testing.T) {
	isolateNpmEnv(t)
	root := t.TempDir()
	t.Chdir(root)

	// package.json + yarn.lock + pnpm-lock.yaml + bun.lock (NO package-lock.json)
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"all-js-test"}`), 0644))
	test.Ok(t, os.WriteFile("yarn.lock", []byte("# yarn lockfile v1\n"), 0644))
	test.Ok(t, os.WriteFile("pnpm-lock.yaml", []byte("lockfileVersion: '9.0'\n"), 0644))
	test.Ok(t, os.WriteFile("bun.lock", []byte("bun-text-lockfile"), 0644))

	_, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// node is NOT detected (package.json fallback excluded by yarn.lock, pnpm-lock.yaml, bun.lock)
	test.Equals(t, []string{"yarn", "pnpm", "bun"}, buildToolsDetected)
}

func TestScenario_BunTextAndBinaryBothPresent(t *testing.T) {
	isolateNpmEnv(t)
	root := t.TempDir()
	t.Chdir(root)

	// Both bun.lock and bun.lockb present
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"bun-test"}`), 0644))
	test.Ok(t, os.WriteFile("bun.lock", []byte("bun-text-lockfile"), 0644))
	test.Ok(t, os.WriteFile("bun.lockb", []byte("bun-binary-lockfile"), 0644))

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// Only bun is detected once
	test.Equals(t, []string{"bun"}, buildToolsDetected)
	test.Equals(t, []string{
		filepath.Join(root, ".cache", "bun"),
		filepath.Join(root, "node_modules"),
	}, directoriesToCache)
}

func TestScenario_AllBuildToolsPresent(t *testing.T) {
	isolateNpmEnv(t)
	isolatePythonEnv(t)
	root := t.TempDir()
	t.Chdir(root)

	allFiles := map[string]string{
		"pom.xml":           "<project></project>",
		"build.gradle":      "plugins {}",
		"build.sbt.lock":    "sbt-lock",
		"MODULE.bazel":      "module()",
		"package.json":      `{"name":"polyglot"}`,
		"package-lock.json": `{"lockfileVersion":3}`,
		"yarn.lock":         "# yarn lockfile v1\n",
		"pnpm-lock.yaml":    "lockfileVersion: '9.0'\n",
		"bun.lock":          "bun.lock",
		"bun.lockb":         "bun.lockb",
		"go.mod":            "module example.com/all\n\ngo 1.21\n",
		"app.csproj":        "<Project></Project>",
		"composer.lock":     `{"packages":[]}`,
		"poetry.lock":       "# poetry lock",
		"uv.lock":           "version = 1",
		"Gemfile":           "source 'https://rubygems.org'",
		"Podfile":           "platform :ios, '14.0'",
		"Package.swift":     "// swift-tools-version:5.5",
		"Cargo.lock":        "version = 3",
	}

	for f, content := range allFiles {
		test.Ok(t, os.WriteFile(f, []byte(content), 0644))
	}

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// Ensure all tool types detected without crashing or duplicating
	expectedTools := []string{
		"maven", "gradle", "sbt", "bazel", "node", "yarn", "pnpm", "bun",
		"golang", "dotnet", "composer", "python-poetry", "python-uv",
		"fastlane", "cocoapods", "spm", "cargo",
	}
	test.Equals(t, expectedTools, buildToolsDetected)
	test.Assert(t, len(directoriesToCache) > 15, "expected many cache directories")
	test.Assert(t, len(hashes) > 0, "expected hashes to be generated")
}
