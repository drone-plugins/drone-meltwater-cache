package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

const (
	pomFile            = "pom.xml"
	nestedDirectory    = "dir"
	bazelBuildFile     = "build.gradle"
	gradleKtsBuildFile = "build.gradle.kts"
	testFileContent    = "some_content"
	testFileContent2   = "some_other_content"
	toolMaven          = "maven"
	toolMavenDir       = ".m2/repository"
	toolGradle         = "gradle"
	toolGradleDir      = ".gradle"
	packageJSONFile    = "package.json"
	packageLockFile    = "package-lock.json"
	npmrcFile          = ".npmrc"
	yarnLockFile       = "yarn.lock"
	toolNode           = "node"
	toolYarn           = "yarn"
)

func TestDetectDirectoriesToCacheMaven(t *testing.T) {
	f, err := os.Create(pomFile)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)
	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(pomFile))
	path, _ := filepath.Abs(toolMavenDir)
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{toolMaven}
	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

func TestDetectDirectoriesToCacheMavenMultiMaven(t *testing.T) {
	f, err := os.Create(pomFile)
	test.Ok(t, err)
	defer f.Close()

	_, err = f.WriteString(testFileContent)

	test.Ok(t, err)
	test.Ok(t, os.MkdirAll(nestedDirectory, 0755))

	f2, err := os.Create(filepath.Join(nestedDirectory, pomFile))

	test.Ok(t, err)
	defer f2.Close()

	_, err = f2.WriteString(testFileContent2)

	test.Ok(t, err)
	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)

	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(pomFile))
	test.Ok(t, os.RemoveAll(filepath.Join(nestedDirectory, pomFile)))

	path, _ := filepath.Abs(toolMavenDir)
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{toolMaven}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

func TestDetectDirectoriesToCacheBazel(t *testing.T) {
	f, err := os.Create(bazelBuildFile)
	test.Ok(t, err)
	defer f.Close()

	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)

	test.Ok(t, os.RemoveAll(bazelBuildFile))
	test.Ok(t, err)

	// Gradle preparer now returns absolute path
	gradlePath, _ := filepath.Abs(toolGradleDir)
	expectedCacheDir := []string{gradlePath}
	expectedDetectedTool := []string{toolGradle}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

func TestDetectDirectoriesToCacheGradleKts(t *testing.T) {
	f, err := os.Create(gradleKtsBuildFile)
	test.Ok(t, err)
	defer f.Close()

	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)

	test.Ok(t, os.RemoveAll(gradleKtsBuildFile))
	test.Ok(t, err)

	// Gradle preparer now returns absolute path
	gradlePath, _ := filepath.Abs(toolGradleDir)
	expectedCacheDir := []string{gradlePath}
	expectedDetectedTool := []string{toolGradle}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

func TestDetectDirectoriesToCacheDotnet(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Unsetenv("NUGET_PACKAGES")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	csprojFile := "test.csproj"
	f, err := os.Create(csprojFile)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(csprojFile))
	test.Ok(t, os.RemoveAll("nuget.config"))

	path, _ := filepath.Abs(".nuget/packages")
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{"dotnet"}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

func TestDetectDirectoriesToCacheDotnetWithEnvVar(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Setenv("NUGET_PACKAGES", "/custom/dotnet/cache")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	csprojFile := "test.csproj"
	f, err := os.Create(csprojFile)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(csprojFile))

	expectedCacheDir := []string{"/custom/dotnet/cache"}
	expectedDetectedTool := []string{"dotnet"}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
}

func TestDetectDirectoriesToCacheCombined(t *testing.T) {
	f, err := os.Create(bazelBuildFile)
	test.Ok(t, err)
	defer f.Close()

	_, err = f.WriteString(testFileContent)

	test.Ok(t, err)
	f2, err := os.Create(pomFile)

	test.Ok(t, err)
	defer f2.Close()

	_, err = f2.WriteString(testFileContent2)

	test.Ok(t, err)
	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)

	test.Ok(t, os.RemoveAll(bazelBuildFile))
	test.Ok(t, os.RemoveAll(pomFile))
	test.Ok(t, err)

	path1, _ := filepath.Abs(toolMavenDir)
	// Gradle preparer now returns absolute path
	gradlePath, _ := filepath.Abs(toolGradleDir)
	expectedCacheDir := []string{path1, gradlePath}
	expectedDetectedTool := []string{toolMaven, toolGradle}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "1eb00e74bffac0c4fa2d6dbfd8c26cb7baab6c16d9143523b7865d46896e4596")
}

// --- New tests for per-project .NET auto-detection ---

func TestHashAllFilesPerProjectIfExistNoMatches(t *testing.T) {
	hash, dirs, err := hashAllFilesPerProjectIfExist("no-such-*.csproj")
	test.Ok(t, err)
	test.Equals(t, "", hash)
	test.Assert(t, dirs == nil, "expected nil dirs for no matches, got %v", dirs)
}

func TestCalculateMd5FromAllFilesPerProject(t *testing.T) {
	dir, err := os.MkdirTemp("", "md5all-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	p1 := filepath.Join(dir, "p1")
	p2 := filepath.Join(dir, "p2")
	test.Ok(t, os.MkdirAll(p1, 0755))
	test.Ok(t, os.MkdirAll(p2, 0755))

	f1 := filepath.Join(p1, "a.csproj")
	f2 := filepath.Join(p2, "b.csproj")
	content1 := []byte("proj1content")
	content2 := []byte("proj2content")
	test.Ok(t, os.WriteFile(f1, content1, 0644))
	test.Ok(t, os.WriteFile(f2, content2, 0644))

	hash, dirs, err := calculateMd5FromAllFilesPerProject([]string{f1, f2})
	test.Ok(t, err)

	// compute expected manually (contents are concatenated in sorted path order)
	h := md5.New() // #nosec
	_, _ = h.Write(content1)
	_, _ = h.Write(content2)
	expectedHash := hex.EncodeToString(h.Sum(nil))
	test.Equals(t, expectedHash, hash)

	absP1, _ := filepath.Abs(p1)
	absP2, _ := filepath.Abs(p2)
	expectedDirs := []string{absP1, absP2}
	test.Equals(t, expectedDirs, dirs)

	// Hardening check: calling with reversed input must produce identical hash and dirs
	// (proves we are no longer sensitive to filepath.Glob order).
	hashRev, dirsRev, err := calculateMd5FromAllFilesPerProject([]string{f2, f1})
	test.Ok(t, err)
	test.Equals(t, hash, hashRev)
	test.Equals(t, dirs, dirsRev)

	// error path
	_, _, err = calculateMd5FromAllFilesPerProject([]string{filepath.Join(dir, "nope.csproj")})
	test.NotOk(t, err)
}

func TestDetectDirectoriesToCacheDotnetMultiProject(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Unsetenv("NUGET_PACKAGES")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	csprojA := "A.csproj"
	csprojB := "B.csproj"
	f, err := os.Create(csprojA)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	f2, err := os.Create(csprojB)
	test.Ok(t, err)
	defer f2.Close()
	_, err = f2.WriteString(testFileContent2)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// compute expected using the same helper and Glob order seen by detection
	matches, _ := filepath.Glob("*.csproj")
	expectedHash, _, _ := calculateMd5FromAllFilesPerProject(matches)

	test.Ok(t, os.RemoveAll(csprojA))
	test.Ok(t, os.RemoveAll(csprojB))
	test.Ok(t, os.RemoveAll("nuget.config"))

	path, _ := filepath.Abs(".nuget/packages")
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{"dotnet"}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, expectedHash)
}

func TestDetectDirectoriesToCacheDotnetFsprojOnly(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Unsetenv("NUGET_PACKAGES")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	fsproj := "lib.fsproj"
	f, err := os.Create(fsproj)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(fsproj))
	test.Ok(t, os.RemoveAll("nuget.config"))

	path, _ := filepath.Abs(".nuget/packages")
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{"dotnet"}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
}

func TestDetectDirectoriesToCacheDotnetMixedProjectTypes(t *testing.T) {
	origEnv := os.Getenv("NUGET_PACKAGES")
	os.Unsetenv("NUGET_PACKAGES")
	defer os.Setenv("NUGET_PACKAGES", origEnv)

	csproj := "app.csproj"
	fsproj := "lib.fsproj"
	f, err := os.Create(csproj)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	f2, err := os.Create(fsproj)
	test.Ok(t, err)
	defer f2.Close()
	_, err = f2.WriteString(testFileContent2)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// compute expected from the csproj glob only (first one processed; later dotnet globs skipped)
	matches, _ := filepath.Glob("*.csproj")
	expectedHash, _, _ := calculateMd5FromAllFilesPerProject(matches)

	test.Ok(t, os.RemoveAll(csproj))
	test.Ok(t, os.RemoveAll(fsproj))
	test.Ok(t, os.RemoveAll("nuget.config"))

	path, _ := filepath.Abs(".nuget/packages")
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{"dotnet"}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, expectedHash)
}

// npm exports npm_config_* to child processes, so these are often already set
// in real shells and CI. Left alone, they would decide the assertions for us.
// HOME is pinned outside the workspace so the default ~/.npm is not treated as
// a workspace cache; the HOME=/workspace case has its own test.
func isolateNpmEnv(t *testing.T) {
	t.Helper()
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HARNESS_WORKSPACE", "")
}

// they don't decide these tests' assertions for us.
func isolatePythonEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PIP_CACHE_DIR", "")
	t.Setenv("PIPENV_CACHE_DIR", "")
	t.Setenv("POETRY_CACHE_DIR", "")
	t.Setenv("UV_CACHE_DIR", "")
}


func TestDetectDirectoriesToCacheNodeUsesPackageLock(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	defer os.Remove(npmrcFile)

	directoriesToCache, buildToolsDetected, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	// No cache= and HOME is outside the workspace, so only node_modules is shared.
	test.Equals(t, directoriesToCache, []string{filepath.Join(workspace, "node_modules")})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	test.Equals(t, hash, "baab6c16d9143523b7865d46896e4596")

	_, err = os.Stat(npmrcFile)
	test.Assert(t, os.IsNotExist(err), "detecting a lockfile must not create .npmrc")
}

func TestDetectDirectoriesToCacheNodeModulesBesideNestedPackageLock(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.MkdirAll(nestedDirectory, 0755))
	defer os.RemoveAll(nestedDirectory)
	test.Ok(t, os.WriteFile(filepath.Join(nestedDirectory, packageLockFile), []byte(testFileContent), 0644))

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	nested, err := filepath.Abs(nestedDirectory)
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(nested, "node_modules")})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	_, err = os.Stat(filepath.Join(nestedDirectory, npmrcFile))
	test.Assert(t, os.IsNotExist(err), "nested lockfile detection must not create .npmrc")
}

func TestDetectDirectoriesToCacheNodeFallsBackToPackageJSON(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageJSONFile, []byte(testFileContent), 0644))
	defer os.Remove(packageJSONFile)

	directoriesToCache, buildToolsDetected, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(workspace, "node_modules")})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	test.Equals(t, hash, md5Hex(t, testFileContent))

	_, err = os.Stat(npmrcFile)
	test.Assert(t, os.IsNotExist(err), "package.json fallback must not write .npmrc")
}

func TestDetectDirectoriesToCacheNodePrefersPackageLock(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageJSONFile, []byte(testFileContent2), 0644))
	defer os.Remove(packageJSONFile)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	defer os.Remove(npmrcFile)

	directoriesToCache, buildToolsDetected, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(workspace, "node_modules")})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	test.Equals(t, hash, md5Hex(t, testFileContent))
}

func TestDetectDirectoriesToCacheNodeModulesBesideNestedPackageJSON(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.MkdirAll(nestedDirectory, 0755))
	defer os.RemoveAll(nestedDirectory)
	test.Ok(t, os.WriteFile(
		filepath.Join(nestedDirectory, packageJSONFile),
		[]byte(testFileContent),
		0644,
	))

	directoriesToCache, buildToolsDetected, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	nested, err := filepath.Abs(nestedDirectory)
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(nested, "node_modules")})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	test.Equals(t, hash, md5Hex(t, testFileContent))
}

func TestDetectDirectoriesToCacheNodeFallbackIgnoresOtherPackageManagers(t *testing.T) {
	for _, lockfile := range []string{"yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb"} {
		t.Run(lockfile, func(t *testing.T) {
			isolateNpmEnv(t)
			dir := t.TempDir()
			t.Chdir(dir)
			test.Ok(t, os.WriteFile(packageJSONFile, []byte(testFileContent), 0644))
			test.Ok(t, os.WriteFile(lockfile, []byte(testFileContent2), 0644))

			directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)

			if lockfile == yarnLockFile {
				test.Equals(t, buildToolsDetected, []string{toolYarn})
				test.Assert(t, len(directoriesToCache) == 2, "expected yarn cache paths")
				return
			}

			test.Assert(t, directoriesToCache == nil, "expected no npm cache paths, got %v", directoriesToCache)
			test.Assert(t, buildToolsDetected == nil, "expected no detected tools, got %v", buildToolsDetected)
		})
	}
}

func TestDetectDirectoriesToCacheNodeFallbackDoesNotUseNestedPackageJSONWhenRootIsBlocked(t *testing.T) {
	for _, lockfile := range []string{"yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb"} {
		t.Run(lockfile, func(t *testing.T) {
			isolateNpmEnv(t)
			dir := t.TempDir()
			t.Chdir(dir)
			test.Ok(t, os.WriteFile(packageJSONFile, []byte(testFileContent), 0644))
			test.Ok(t, os.WriteFile(lockfile, []byte(testFileContent2), 0644))
			test.Ok(t, os.MkdirAll(nestedDirectory, 0755))
			test.Ok(t, os.WriteFile(
				filepath.Join(nestedDirectory, packageJSONFile),
				[]byte(testFileContent),
				0644,
			))

			directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)

			fallback, err := NpmPackageJSONFallbackDetected()
			test.Ok(t, err)
			test.Assert(t, !fallback, "expected no package.json fallback when a root lockfile blocks it")

			if lockfile == yarnLockFile {
				test.Equals(t, buildToolsDetected, []string{toolYarn})
				test.Assert(t, len(directoriesToCache) == 2, "expected yarn cache paths")
				return
			}

			test.Assert(t, directoriesToCache == nil, "expected no npm cache paths, got %v", directoriesToCache)
			test.Assert(t, buildToolsDetected == nil, "expected no detected tools, got %v", buildToolsDetected)
		})
	}
}

func TestDetectDirectoriesToCacheNodeLockfileChangeInvalidatesKey(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	defer os.Remove(npmrcFile)

	_, _, firstHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent2), 0644))
	_, _, secondHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, firstHash != secondHash, "expected package-lock.json change to invalidate cache key")
}

// Restore and save both run detection in the same workspace. Neither run may
// create or rewrite .npmrc.
func TestDetectDirectoriesToCacheNodeIsIdempotentAcrossSteps(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)

	firstDirs, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	secondDirs, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, firstDirs, secondDirs)
	_, err = os.Stat(npmrcFile)
	test.Assert(t, os.IsNotExist(err), "repeated detection must not create .npmrc")
}

// Appending our own entry would silently move the user's cache, so theirs wins.
func TestDetectDirectoriesToCacheNodeRespectsExistingNpmrcCache(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	test.Ok(t, os.WriteFile(npmrcFile, []byte("registry=https://example.com\ncache=custom-npm-cache\n"), 0644))
	defer os.Remove(npmrcFile)

	directoriesToCache, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{
		filepath.Join(workspace, "node_modules"),
		filepath.Join(workspace, "custom-npm-cache"),
	})

	npmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)
	test.Equals(t, string(npmrc), "registry=https://example.com\ncache=custom-npm-cache\n")
}

// npm_config_cache overrides .npmrc, so the env var path is what gets cached.
func TestDetectDirectoriesToCacheNodeHonoursNpmConfigCacheEnv(t *testing.T) {
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)

	envCache, err := filepath.Abs("env-npm-cache")
	test.Ok(t, err)
	t.Setenv("npm_config_cache", envCache)

	directoriesToCache, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(workspace, "node_modules"), envCache})

	_, err = os.Stat(npmrcFile)
	test.Assert(t, os.IsNotExist(err), "expected no .npmrc to be written when npm_config_cache is set")
}

// A tracked project .npmrc with no cache= must be left byte-for-byte alone.
// Appending cache=/harness/.npm makes `npm version` fail its clean-tree check.
func TestDetectDirectoriesToCacheNodeDoesNotMutateTrackedNpmrc(t *testing.T) {
	isolateNpmEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)

	const original = "registry=https://example.com/npm/\n//example.com/npm/:_authToken=${TOKEN}\nca=null\n"
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	test.Ok(t, os.WriteFile(npmrcFile, []byte(original), 0644))

	directoriesToCache, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(workspace, "node_modules")})

	npmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)
	test.Equals(t, string(npmrc), original)
}

// When HOME is the checkout, npm already uses <workspace>/.npm. Cache that
// directory, but do not write the setting into the tracked .npmrc.
func TestDetectDirectoriesToCacheNodeHomeEqualsWorkspaceDoesNotMutateNpmrc(t *testing.T) {
	isolateNpmEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)

	const original = "registry=https://example.com/npm/\n//example.com/npm/:_authToken=${TOKEN}\nca=null\n"
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	test.Ok(t, os.WriteFile(npmrcFile, []byte(original), 0644))

	directoriesToCache, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{
		filepath.Join(workspace, "node_modules"),
		filepath.Join(workspace, npmCacheDirName),
	})

	npmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)
	test.Equals(t, string(npmrc), original)
}

// .npmrc is sometimes a read-only mounted secret. Losing the tarball cache is
// fine; failing the whole step is not.
func TestDetectDirectoriesToCacheNodeSurvivesUnwritableNpmrc(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	test.Ok(t, os.WriteFile(npmrcFile, []byte("registry=https://example.com\n"), 0444))
	defer os.Remove(npmrcFile)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{filepath.Join(workspace, "node_modules")})
	test.Equals(t, buildToolsDetected, []string{toolNode})

	// Left exactly as it was.
	npmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)
	test.Equals(t, string(npmrc), "registry=https://example.com\n")
}

// md5Hex is what one file contributes to the cache key.
func md5Hex(t *testing.T, content string) string {
	t.Helper()

	h := md5.New() // #nosec
	_, err := h.Write([]byte(content))
	test.Ok(t, err)

	return hex.EncodeToString(h.Sum(nil))
}

// Yarn repos matched the old package.json glob too, so their key was
// md5(package.json) then md5(yarn.lock). It has to stay byte-identical, or
// every yarn repo takes a miss on upgrade.
func TestDetectDirectoriesToCacheYarnKeyIncludesPackageJSON(t *testing.T) {
	test.Ok(t, os.WriteFile(packageJSONFile, []byte(testFileContent), 0644))
	defer os.Remove(packageJSONFile)
	test.Ok(t, os.WriteFile(yarnLockFile, []byte(testFileContent2), 0644))
	defer os.Remove(yarnLockFile)
	defer os.Remove(".yarnrc")
	defer os.Remove(".yarnrc.yaml")

	_, buildToolsDetected, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, buildToolsDetected, []string{toolYarn})
	test.Equals(t, hash, md5Hex(t, testFileContent)+md5Hex(t, testFileContent2))
}

func TestDetectDirectoriesToCacheYarnKeyChangesWithPackageJSON(t *testing.T) {
	test.Ok(t, os.WriteFile(packageJSONFile, []byte(testFileContent), 0644))
	defer os.Remove(packageJSONFile)
	test.Ok(t, os.WriteFile(yarnLockFile, []byte(testFileContent2), 0644))
	defer os.Remove(yarnLockFile)
	defer os.Remove(".yarnrc")
	defer os.Remove(".yarnrc.yaml")

	_, _, firstHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Ok(t, os.WriteFile(packageJSONFile, []byte("changed"), 0644))
	_, _, secondHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, firstHash != secondHash, "expected package.json change to invalidate the yarn cache key")
}

// No package.json means only yarn.lock contributes, as before - there was
// nothing for the old glob to match either.
func TestDetectDirectoriesToCacheYarnKeyWithoutPackageJSON(t *testing.T) {
	test.Ok(t, os.WriteFile(yarnLockFile, []byte(testFileContent), 0644))
	defer os.Remove(yarnLockFile)
	defer os.Remove(".yarnrc")
	defer os.Remove(".yarnrc.yaml")

	_, _, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, hash, md5Hex(t, testFileContent))
}

// The old package.json glob is what cached node_modules for yarn repos.
func TestDetectDirectoriesToCacheYarnStillCachesNodeModules(t *testing.T) {
	test.Ok(t, os.WriteFile(yarnLockFile, []byte(testFileContent), 0644))
	defer os.Remove(yarnLockFile)
	defer os.Remove(".yarnrc")
	defer os.Remove(".yarnrc.yaml")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	test.Equals(t, directoriesToCache, []string{
		filepath.Join(workspace, ".yarn"),
		filepath.Join(workspace, "node_modules"),
	})
	test.Equals(t, buildToolsDetected, []string{toolYarn})
}

func TestDetectDirectoriesToCachePythonPoetry(t *testing.T) {
	isolatePythonEnv(t)
	f, err := os.Create("poetry.lock")
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	// Poetry requires pyproject.toml
	fp, err := os.Create("pyproject.toml")
	test.Ok(t, err)
	defer fp.Close()
	_, err = fp.WriteString("[tool.poetry]\nname = \"test\"\n")
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll("poetry.lock"))
	test.Ok(t, os.RemoveAll("pyproject.toml"))
	test.Ok(t, os.RemoveAll("poetry.toml"))

	// Should detect python tool
	test.Assert(t, len(buildToolsDetected) > 0, "expected at least one tool detected")
	test.Assert(t, containsTool(buildToolsDetected, "python-poetry"), "expected python-poetry in detected tools, got %v", buildToolsDetected)
	test.Equals(t, len(directoriesToCache) > 0, true)
	// Find the poetry cache dir
	var poetryCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "poetry" {
			poetryCacheFound = true
			break
		}
	}
	test.Assert(t, poetryCacheFound, "expected poetry cache dir in %v", directoriesToCache)
}

func TestDetectDirectoriesToCachePythonPipfile(t *testing.T) {
	isolatePythonEnv(t)
	f, err := os.Create("Pipfile.lock")
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll("Pipfile.lock"))
	test.Ok(t, os.RemoveAll(".env"))

	// Python should be detected
	test.Assert(t, len(buildToolsDetected) > 0, "expected at least one tool detected")
	test.Assert(t, containsTool(buildToolsDetected, "python-pipenv"), "expected python-pipenv in detected tools, got %v", buildToolsDetected)
	test.Equals(t, len(directoriesToCache) > 0, true)
	// Find the pipenv cache dir
	var pipenvCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "pipenv" {
			pipenvCacheFound = true
			break
		}
	}
	test.Assert(t, pipenvCacheFound, "expected pipenv cache dir in %v", directoriesToCache)
}

func TestDetectDirectoriesToCachePythonRequirements(t *testing.T) {
	isolatePythonEnv(t)
	f, err := os.Create("requirements.txt")
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll("requirements.txt"))
	test.Ok(t, os.RemoveAll("pip.conf"))

	// Python should be detected
	test.Assert(t, len(buildToolsDetected) > 0, "expected at least one tool detected")
	test.Assert(t, containsTool(buildToolsDetected, "python-pip"), "expected python-pip in detected tools, got %v", buildToolsDetected)
	test.Equals(t, len(directoriesToCache) > 0, true)
	// Find the pip cache dir
	var pipCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "pip" {
			pipCacheFound = true
			break
		}
	}
	test.Assert(t, pipCacheFound, "expected pip cache dir in %v", directoriesToCache)
}

func TestDetectDirectoriesToCachePythonRequirementsWithoutEnv(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("requirements.txt", []byte(testFileContent), 0644))
	defer os.Remove("requirements.txt")
	defer os.Remove("pip.conf")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, containsTool(buildToolsDetected, "python-pip"),
		"expected python-pip detection without PIP_CACHE_DIR, got %v", buildToolsDetected)
	var pipCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "pip" {
			pipCacheFound = true
		}
	}
	test.Assert(t, pipCacheFound, "expected pip cache dir in %v", directoriesToCache)
}

func TestDetectDirectoriesToCachePythonPoetryPriority(t *testing.T) {
	isolatePythonEnv(t)
	// Create both poetry.lock and requirements.txt
	f1, err := os.Create("poetry.lock")
	test.Ok(t, err)
	defer f1.Close()
	_, err = f1.WriteString(testFileContent)
	test.Ok(t, err)

	// Poetry requires pyproject.toml
	fp, err := os.Create("pyproject.toml")
	test.Ok(t, err)
	defer fp.Close()
	_, err = fp.WriteString("[tool.poetry]\nname = \"test\"\n")
	test.Ok(t, err)

	f2, err := os.Create("requirements.txt")
	test.Ok(t, err)
	defer f2.Close()
	_, err = f2.WriteString(testFileContent2)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll("poetry.lock"))
	test.Ok(t, os.RemoveAll("pyproject.toml"))
	test.Ok(t, os.RemoveAll("requirements.txt"))
	test.Ok(t, os.RemoveAll("poetry.toml"))
	test.Ok(t, os.RemoveAll("pip.conf"))

	// Should detect python tool
	test.Assert(t, len(buildToolsDetected) > 0, "expected at least one tool detected")
	test.Assert(t, containsTool(buildToolsDetected, "python-poetry"), "expected python-poetry in detected tools, got %v", buildToolsDetected)
	test.Assert(t, containsTool(buildToolsDetected, "python-pip"), "expected python-pip in detected tools, got %v", buildToolsDetected)
	// Should find poetry cache dir
	var poetryCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "poetry" {
			poetryCacheFound = true
			break
		}
	}
	test.Assert(t, poetryCacheFound, "expected poetry cache dir, got %v", directoriesToCache)
}

func TestDetectDirectoriesToCacheUv(t *testing.T) {
	isolatePythonEnv(t)
	// Create uv.lock and pyproject.toml
	f, err := os.Create("uv.lock")
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	fp, err := os.Create("pyproject.toml")
	test.Ok(t, err)
	defer fp.Close()
	_, err = fp.WriteString("[project]\nname = \"test\"\n")
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll("uv.lock"))
	test.Ok(t, os.RemoveAll("pyproject.toml"))

	// uv should be detected as python tool
	test.Assert(t, len(buildToolsDetected) > 0, "expected at least one tool detected")
	test.Assert(t, containsTool(buildToolsDetected, "python-uv"), "expected python-uv in detected tools, got %v", buildToolsDetected)
	test.Equals(t, len(directoriesToCache) > 0, true)
	// Find the uv cache dir
	var uvCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "uv" {
			uvCacheFound = true
			break
		}
	}
	test.Assert(t, uvCacheFound, "expected uv cache dir in %v", directoriesToCache)
}

func TestDetectDirectoriesToCacheUvPriority(t *testing.T) {
	isolatePythonEnv(t)
	// Create uv.lock and Pipfile.lock - both should be detected
	f1, err := os.Create("uv.lock")
	test.Ok(t, err)
	defer f1.Close()
	_, err = f1.WriteString(testFileContent)
	test.Ok(t, err)

	fp, err := os.Create("pyproject.toml")
	test.Ok(t, err)
	defer fp.Close()
	_, err = fp.WriteString("[project]\nname = \"test\"\n")
	test.Ok(t, err)

	f2, err := os.Create("Pipfile.lock")
	test.Ok(t, err)
	defer f2.Close()
	_, err = f2.WriteString(testFileContent2)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll("uv.lock"))
	test.Ok(t, os.RemoveAll("pyproject.toml"))
	test.Ok(t, os.RemoveAll("Pipfile.lock"))

	// Should detect both python tools
	test.Assert(t, len(buildToolsDetected) > 0, "expected at least one tool detected")
	test.Assert(t, containsTool(buildToolsDetected, "python-uv"), "expected python-uv in detected tools, got %v", buildToolsDetected)
	test.Assert(t, containsTool(buildToolsDetected, "python-pipenv"), "expected python-pipenv in detected tools, got %v", buildToolsDetected)
	// Find uv cache dir
	var uvCacheFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "uv" {
			uvCacheFound = true
			break
		}
	}
	test.Assert(t, uvCacheFound, "expected uv cache dir, got %v", directoriesToCache)
}

// cacheDirNames reduces absolute cache paths to their leaf names so assertions
// can name the managers involved instead of full temp paths.
func cacheDirNames(dirs []string) []string {
	names := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		names = append(names, filepath.Base(dir))
	}
	return names
}

func assertCachesManagers(t *testing.T, dirs []string, want ...string) {
	t.Helper()
	names := cacheDirNames(dirs)
	for _, manager := range want {
		test.Assert(t, containsTool(names, manager),
			"expected %s cache dir, got %v", manager, names)
	}
}

// A repo migrating to uv keeps requirements.txt around, so both caches are needed.
func TestDetectDirectoriesToCachePythonUvAndRequirementsCoexist(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("uv.lock", []byte(testFileContent), 0644))
	defer os.Remove("uv.lock")
	test.Ok(t, os.WriteFile("requirements.txt", []byte(testFileContent2), 0644))
	defer os.Remove("requirements.txt")
	defer os.Remove("pip.conf")
	defer os.Remove("uv.toml")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-uv", "python-pip"}, buildToolsDetected)
	assertCachesManagers(t, directoriesToCache, "uv", "pip")
}

func TestDetectDirectoriesToCachePythonPoetryAndRequirementsCoexist(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("poetry.lock", []byte(testFileContent), 0644))
	defer os.Remove("poetry.lock")
	test.Ok(t, os.WriteFile("requirements.txt", []byte(testFileContent2), 0644))
	defer os.Remove("requirements.txt")
	defer os.Remove("pip.conf")
	defer os.Remove("poetry.toml")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-poetry", "python-pip"}, buildToolsDetected)
	assertCachesManagers(t, directoriesToCache, "poetry", "pip")
}

func TestDetectDirectoriesToCachePythonPoetryAndPipenvCoexist(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("poetry.lock", []byte(testFileContent), 0644))
	defer os.Remove("poetry.lock")
	test.Ok(t, os.WriteFile("Pipfile.lock", []byte(testFileContent2), 0644))
	defer os.Remove("Pipfile.lock")
	defer os.Remove("poetry.toml")
	defer os.Remove(".env")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-poetry", "python-pipenv"}, buildToolsDetected)
	assertCachesManagers(t, directoriesToCache, "poetry", "pipenv")
}

func TestDetectDirectoriesToCachePythonPipenvAndRequirementsCoexist(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("Pipfile.lock", []byte(testFileContent), 0644))
	defer os.Remove("Pipfile.lock")
	test.Ok(t, os.WriteFile("requirements.txt", []byte(testFileContent2), 0644))
	defer os.Remove("requirements.txt")
	defer os.Remove("pip.conf")
	defer os.Remove(".env")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"python-pipenv", "python-pip"}, buildToolsDetected)
	assertCachesManagers(t, directoriesToCache, "pipenv", "pip")
}

// requirements.txt and constraints.txt both configure pip, so the shared
// manager must be prepared once rather than contributing a duplicate entry.
func TestDetectDirectoriesToCachePythonPipPreparedOncePerRepo(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("requirements.txt", []byte(testFileContent), 0644))
	defer os.Remove("requirements.txt")
	test.Ok(t, os.WriteFile("constraints.txt", []byte(testFileContent2), 0644))
	defer os.Remove("constraints.txt")
	defer os.Remove("pip.conf")

	directoriesToCache, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	pipPath, err := filepath.Abs(filepath.Join(".cache", "pip"))
	test.Ok(t, err)
	test.Equals(t, []string{pipPath}, directoriesToCache)
}

// MODULE.bazel supersedes WORKSPACE, so the two preparers must not both run.
func TestDetectDirectoriesToCacheBazelPrefersBzlmod(t *testing.T) {
	test.Ok(t, os.WriteFile("MODULE.bazel", []byte(testFileContent), 0644))
	defer os.Remove("MODULE.bazel")
	test.Ok(t, os.WriteFile("WORKSPACE", []byte(testFileContent2), 0644))
	defer os.Remove("WORKSPACE")
	defer os.Remove(".bazelrc")
	defer os.Remove(".bazelignore")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"bazel"}, buildToolsDetected)
	test.Equals(t, 1, len(directoriesToCache))
}

func TestDetectDirectoriesToCachePythonConstraints(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("constraints.txt", []byte("requests==2.0.0\n"), 0644))
	defer os.Remove("constraints.txt")
	defer os.Remove("pip.conf")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, containsTool(buildToolsDetected, "python-pip"), "expected python-pip in detected tools, got %v", buildToolsDetected)
	var pipFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "pip" {
			pipFound = true
		}
	}
	test.Assert(t, pipFound, "expected pip cache dir for constraints.txt in %v", directoriesToCache)
}

func TestDetectDirectoriesToCachePythonPyproject(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("pyproject.toml", []byte("[project]\nname = \"demo\"\n"), 0644))
	defer os.Remove("pyproject.toml")
	defer os.Remove("pip.conf")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, containsTool(buildToolsDetected, "python-pip"), "expected python-pip in detected tools, got %v", buildToolsDetected)
	var pipFound bool
	for _, dir := range directoriesToCache {
		if filepath.Base(dir) == "pip" {
			pipFound = true
		}
	}
	test.Assert(t, pipFound, "expected pip cache dir for pyproject.toml in %v", directoriesToCache)
}

func TestDetectDirectoriesToCachePythonPyprojectExcludedWhenPoetryLockExists(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("pyproject.toml", []byte("[project]\nname = \"demo\"\n"), 0644))
	defer os.Remove("pyproject.toml")
	test.Ok(t, os.WriteFile("poetry.lock", []byte(testFileContent), 0644))
	defer os.Remove("poetry.lock")
	defer os.Remove("poetry.toml")
	defer os.Remove("pip.conf")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"python-poetry"}, buildToolsDetected)
	assertCachesManagers(t, directoriesToCache, "poetry")
	test.Assert(t, !containsCacheDir(directoriesToCache, "pip"), "expected pip not to be cached when pyproject.toml is superseded by poetry.lock")
}

func TestDetectDirectoriesToCachePythonPyprojectExcludedWhenUvLockExists(t *testing.T) {
	isolatePythonEnv(t)
	test.Ok(t, os.WriteFile("pyproject.toml", []byte("[project]\nname = \"demo\"\n"), 0644))
	defer os.Remove("pyproject.toml")
	test.Ok(t, os.WriteFile("uv.lock", []byte(testFileContent), 0644))
	defer os.Remove("uv.lock")
	defer os.Remove("uv.toml")
	defer os.Remove("pip.conf")

	directoriesToCache, buildToolsDetected, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"python-uv"}, buildToolsDetected)
	assertCachesManagers(t, directoriesToCache, "uv")
	test.Assert(t, !containsCacheDir(directoriesToCache, "pip"), "expected pip not to be cached when pyproject.toml is superseded by uv.lock")
}

func containsCacheDir(dirs []string, name string) bool {
	for _, d := range dirs {
		if filepath.Base(d) == name {
			return true
		}
	}
	return false
}
