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
