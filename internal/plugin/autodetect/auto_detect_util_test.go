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

func TestHashFirstMatchPerProjectNoMatches(t *testing.T) {
	hash, dirs, err := hashFirstMatchPerProject([]string{"no-such-*.csproj"}, nil)
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

// --- iOS auto-detection (CocoaPods / SPM / Fastlane) ---

const (
	hashOfContent1 = "baab6c16d9143523b7865d46896e4596" // md5(testFileContent)
	hashOfContent2 = "1eb00e74bffac0c4fa2d6dbfd8c26cb7" // md5(testFileContent2)
)

// inTempRepo runs a detection pass inside an empty temporary directory and
// returns its resolved absolute path. The older tests in this file write
// fixtures into the package directory itself, which makes them order-dependent
// and leaves debris in the working tree when they fail.
func inTempRepo(t *testing.T) string {
	t.Helper()

	orig, err := os.Getwd()
	test.Ok(t, err)

	dir := t.TempDir()
	test.Ok(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// On macOS t.TempDir() sits under /var, a symlink to /private/var, while
	// detection reports resolved absolute paths.
	resolved, err := filepath.EvalSymlinks(dir)
	test.Ok(t, err)

	return resolved
}

// isolateIOSEnv gives the test a private HOME and clears every env var the iOS
// preparers consult, so results never depend on the developer's machine.
func isolateIOSEnv(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, key := range []string{
		"CP_CACHE_DIR", "CP_HOME_DIR",
		"BUNDLE_PATH", "BUNDLE_APP_CONFIG",
		"XDG_CACHE_HOME", "NUGET_PACKAGES",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}

	return home
}

func writeRepoFile(t *testing.T, path, contents string) {
	t.Helper()

	if parent := filepath.Dir(path); parent != "." {
		test.Ok(t, os.MkdirAll(parent, 0755))
	}

	test.Ok(t, os.WriteFile(path, []byte(contents), 0644))
}

func podsCacheDir(home string) string {
	return filepath.Join(home, "Library", "Caches", "CocoaPods")
}

func swiftpmCacheDir(home string) string {
	return filepath.Join(home, "Library", "Caches", "org.swift.swiftpm")
}

func TestDetectDirectoriesToCacheCocoapodsManifestOnly(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	root := inTempRepo(t)

	writeRepoFile(t, "Podfile", testFileContent)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cocoapods"}, tools)
	test.Equals(t, []string{filepath.Join(root, "Pods"), podsCacheDir(home)}, dirs)
	test.Equals(t, hashOfContent1, hashes)
}

func TestDetectDirectoriesToCacheCocoapodsWithLock(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	root := inTempRepo(t)

	writeRepoFile(t, "Podfile", testFileContent)
	writeRepoFile(t, "Podfile.lock", testFileContent2)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cocoapods"}, tools)
	test.Equals(t, []string{filepath.Join(root, "Pods"), podsCacheDir(home)}, dirs)
	// Podfile.lock is registered first, so the key comes from the lock file.
	test.Equals(t, hashOfContent2, hashes)
}

func TestDetectDirectoriesToCacheSPMPackageResolvedAtRoot(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, "Package.swift", testFileContent)
	writeRepoFile(t, "Package.resolved", testFileContent2)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"spm"}, tools)
	test.Equals(t, []string{swiftpmCacheDir(home)}, dirs)
	test.Equals(t, hashOfContent2, hashes)
}

// TestDetectDirectoriesToCacheSPMInsideXcodeProject is the regression test for
// Go's filepath.Glob treating "**" as a single path element: an Xcode app that
// adopts SPM through the UI keeps Package.resolved four levels down and has no
// root Package.swift, so the old root + one-level probe detected nothing at all.
func TestDetectDirectoriesToCacheSPMInsideXcodeProject(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, filepath.Join(
		"App.xcodeproj", "project.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved"),
		testFileContent)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"spm"}, tools)
	test.Equals(t, []string{swiftpmCacheDir(home)}, dirs)
	test.Equals(t, hashOfContent1, hashes)
}

func TestDetectDirectoriesToCacheSPMInsideXcodeWorkspace(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, filepath.Join(
		"App.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved"),
		testFileContent)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"spm"}, tools)
	test.Equals(t, []string{swiftpmCacheDir(home)}, dirs)
	test.Equals(t, hashOfContent1, hashes)
}

func TestDetectDirectoriesToCacheSPMInsideNestedXcodeProject(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, filepath.Join(
		"ios", "App.xcodeproj", "project.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved"),
		testFileContent)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"spm"}, tools)
	test.Equals(t, []string{swiftpmCacheDir(home)}, dirs)
	test.Equals(t, hashOfContent1, hashes)
}

func TestDetectDirectoriesToCacheSPMPrefersRootResolvedOverXcode(t *testing.T) {
	isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, "Package.resolved", testFileContent2)
	writeRepoFile(t, filepath.Join(
		"App.xcodeproj", "project.xcworkspace", "xcshareddata", "swiftpm", "Package.resolved"),
		testFileContent)

	_, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"spm"}, tools)
	test.Equals(t, hashOfContent2, hashes)
}

func TestDetectDirectoriesToCacheGemfileWithoutIOSMarkerIsIgnored(t *testing.T) {
	isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, "Gemfile", testFileContent)

	dirs, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, len(tools) == 0, "expected no tools detected, got %v", tools)
	test.Assert(t, len(dirs) == 0, "expected no cache dirs, got %v", dirs)
	test.Equals(t, "", hashes)

	_, err = os.Stat(filepath.Join(".bundle", "config"))
	test.Assert(t, os.IsNotExist(err), "expected no .bundle/config, got err=%v", err)
}

func TestDetectDirectoriesToCacheGemfileWithIOSMarkerInSameDir(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	root := inTempRepo(t)

	writeRepoFile(t, "Gemfile", testFileContent)
	writeRepoFile(t, "Podfile", testFileContent2)

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cocoapods", "fastlane"}, tools)
	test.Equals(t, []string{
		filepath.Join(root, "Pods"),
		podsCacheDir(home),
		filepath.Join(root, "vendor", "bundle"),
	}, dirs)

	data, err := os.ReadFile(filepath.Join(".bundle", "config"))
	test.Ok(t, err)
	test.Equals(t, "---\nBUNDLE_PATH: \"vendor/bundle\"\n", string(data))
}

// TestDetectDirectoriesToCacheGemfileMonorepoDoesNotHijackBackend is the
// regression test for the repo-wide marker check: an ios/Podfile must not switch
// on Fastlane for an unrelated backend Gemfile at the root, because doing so
// rewrote that project's Bundler config.
func TestDetectDirectoriesToCacheGemfileMonorepoDoesNotHijackBackend(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	root := inTempRepo(t)

	writeRepoFile(t, "Gemfile", testFileContent)                        // Rails backend
	writeRepoFile(t, filepath.Join("ios", "Podfile"), testFileContent2) // unrelated iOS app

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cocoapods"}, tools)
	test.Equals(t, []string{filepath.Join(root, "ios", "Pods"), podsCacheDir(home)}, dirs)

	_, err = os.Stat(filepath.Join(".bundle", "config"))
	test.Assert(t, os.IsNotExist(err), "backend Bundler config must be untouched, got err=%v", err)
}

func TestDetectDirectoriesToCacheGemfileNestedIOSApp(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")
	root := inTempRepo(t)

	writeRepoFile(t, filepath.Join("ios", "Gemfile"), testFileContent)
	writeRepoFile(t, filepath.Join("ios", "Podfile"), testFileContent2)

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, []string{"cocoapods", "fastlane"}, tools)
	test.Equals(t, []string{
		filepath.Join(root, "ios", "Pods"),
		podsCacheDir(home),
		filepath.Join(root, "ios", "vendor", "bundle"),
	}, dirs)

	_, err = os.Stat(filepath.Join("ios", ".bundle", "config"))
	test.Ok(t, err)
}

func TestDetectDirectoriesToCacheGemfileLockPrecedence(t *testing.T) {
	isolateIOSEnv(t)
	withGOOS(t, "darwin")
	inTempRepo(t)

	writeRepoFile(t, "Gemfile", testFileContent)
	writeRepoFile(t, "Gemfile.lock", testFileContent2)
	test.Ok(t, os.MkdirAll("App.xcworkspace", 0755))

	_, tools, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	// The bare App.xcworkspace acts as the iOS marker but holds no
	// Package.resolved, so spm stays undetected and only fastlane fires.
	test.Equals(t, []string{"fastlane"}, tools)
	// Gemfile.lock is registered before Gemfile, so it supplies the key.
	test.Equals(t, hashOfContent2, hashes)
}

func TestDirSatisfiesRequires(t *testing.T) {
	inTempRepo(t)

	writeRepoFile(t, filepath.Join("ios", "Podfile"), "")
	writeRepoFile(t, filepath.Join("ios", "Gemfile"), "")
	writeRepoFile(t, "Gemfile", "")

	requires := []string{"Podfile", "Package.swift", "*.xcodeproj", "*.xcworkspace"}

	test.Assert(t, dirSatisfiesRequires(filepath.Join("ios", "Gemfile"), requires),
		"ios/Gemfile sits next to ios/Podfile")
	test.Assert(t, !dirSatisfiesRequires("Gemfile", requires),
		"root Gemfile has no marker in its own directory")
	test.Assert(t, dirSatisfiesRequires("Gemfile", nil),
		"no requires means always satisfied")
}

func TestProbePatternsOrder(t *testing.T) {
	patterns := probePatterns(buildToolInfo{
		globToDetect: "Package.resolved",
		alsoDetect:   []string{filepath.Join("*.xcworkspace", "deep", "Package.resolved")},
	})

	test.Equals(t, []string{
		"Package.resolved",
		filepath.Join("**", "Package.resolved"),
		filepath.Join("*.xcworkspace", "deep", "Package.resolved"),
		filepath.Join("**", "*.xcworkspace", "deep", "Package.resolved"),
	}, patterns)
}

func TestPrepareRepoDispatchesSinglePathPreparer(t *testing.T) {
	dir := t.TempDir()

	paths, err := prepareRepo(newGoPreparer(), dir)
	test.Ok(t, err)
	test.Equals(t, []string{".go"}, paths)
}

func TestPrepareRepoDispatchesMultiPathPreparer(t *testing.T) {
	home := isolateIOSEnv(t)
	withGOOS(t, "darwin")

	dir := t.TempDir()

	paths, err := prepareRepo(newCocoapodsPreparer(), dir)
	test.Ok(t, err)
	test.Equals(t, []string{filepath.Join(dir, "Pods"), podsCacheDir(home)}, paths)
}
