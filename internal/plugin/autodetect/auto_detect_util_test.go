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
func isolateNpmEnv(t *testing.T) {
	t.Helper()
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
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
	// Both have to be inside the workspace, the only shared volume.
	test.Equals(t, directoriesToCache, []string{
		filepath.Join(workspace, "node_modules"),
		filepath.Join(workspace, npmCacheDirName),
	})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	test.Equals(t, hash, "baab6c16d9143523b7865d46896e4596")

	// npm has to be told, or it keeps writing to ~/.npm.
	npmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)
	test.Equals(t, string(npmrc), "cache="+filepath.Join(workspace, npmCacheDirName)+"\n")
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
	test.Equals(t, directoriesToCache, []string{
		filepath.Join(nested, "node_modules"),
		filepath.Join(nested, npmCacheDirName),
	})
	test.Equals(t, buildToolsDetected, []string{toolNode})
	test.Exists(t, filepath.Join(nestedDirectory, npmrcFile))
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
	test.Equals(t, directoriesToCache, []string{
		filepath.Join(workspace, "node_modules"),
		filepath.Join(workspace, npmCacheDirName),
	})
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

// Restore and save both run detection in the same workspace, so the second run
// has to land on the same path without appending a duplicate entry.
func TestDetectDirectoriesToCacheNodeIsIdempotentAcrossSteps(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	defer os.Remove(npmrcFile)

	firstDirs, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	firstNpmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)

	secondDirs, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	secondNpmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)

	test.Equals(t, firstDirs, secondDirs)
	test.Equals(t, string(firstNpmrc), string(secondNpmrc))
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

// Appending must not run into the last line of a user's file (CI-24154).
func TestDetectDirectoriesToCacheNodeAppendsToNpmrcWithoutTrailingNewline(t *testing.T) {
	isolateNpmEnv(t)
	test.Ok(t, os.WriteFile(packageLockFile, []byte(testFileContent), 0644))
	defer os.Remove(packageLockFile)
	test.Ok(t, os.WriteFile(npmrcFile, []byte("registry=https://example.com"), 0644))
	defer os.Remove(npmrcFile)

	_, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	workspace, err := filepath.Abs(".")
	test.Ok(t, err)
	npmrc, err := os.ReadFile(npmrcFile)
	test.Ok(t, err)
	test.Equals(t, string(npmrc),
		"registry=https://example.com\ncache="+filepath.Join(workspace, npmCacheDirName)+"\n")
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
