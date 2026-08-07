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
	yarnLockFile       = "yarn.lock"
	toolNode           = "node"
	toolYarn           = "yarn"
)

func md5Hex(content string) string {
	h := md5.New() // #nosec
	_, _ = h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

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

// --- New tests for npm/yarn auto-detection (CI-24147) ---

// npm caching must key off package-lock.json (the resolved dependency tree),
// not package.json (declared ranges), so a lockfile-only dependency change
// actually invalidates the cache. See CI-18670 / CI-24146 for the evidence.
func TestDetectDirectoriesToCacheNodeLockfile(t *testing.T) {
	f, err := os.Create(packageLockFile)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(packageLockFile))

	path, _ := filepath.Abs("node_modules")
	expectedCacheDir := []string{path}
	expectedDetectedTool := []string{toolNode}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

// package.json alone (no committed lockfile) must NOT trigger node_modules
// caching — a repo without a lockfile has nothing stable to key the cache on.
func TestDetectDirectoriesToCacheNodePackageJSONOnlyNotDetected(t *testing.T) {
	f, err := os.Create(packageJSONFile)
	test.Ok(t, err)
	defer f.Close()
	_, err = f.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(packageJSONFile))

	test.Assert(t, directoriesToCache == nil, "expected no cache dirs, got %v", directoriesToCache)
	test.Assert(t, buildToolsDetected == nil, "expected no tools detected, got %v", buildToolsDetected)
	test.Equals(t, hashes, "")
}

// A yarn project (package.json + yarn.lock, no package-lock.json) must only
// pick up the yarn cache dir — not a redundant/no-op node_modules mount via
// package.json, since package.json is no longer a detection signal on its own.
func TestDetectDirectoriesToCacheYarnOnlyNoNodeModules(t *testing.T) {
	fp, err := os.Create(packageJSONFile)
	test.Ok(t, err)
	defer fp.Close()
	_, err = fp.WriteString(testFileContent2)
	test.Ok(t, err)

	fy, err := os.Create(yarnLockFile)
	test.Ok(t, err)
	defer fy.Close()
	_, err = fy.WriteString(testFileContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(packageJSONFile))
	test.Ok(t, os.RemoveAll(yarnLockFile))
	test.Ok(t, os.RemoveAll(".yarnrc"))
	test.Ok(t, os.RemoveAll(".yarnrc.yaml"))

	yarnPath, _ := filepath.Abs(".yarn")
	expectedCacheDir := []string{yarnPath}
	expectedDetectedTool := []string{toolYarn}

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, "baab6c16d9143523b7865d46896e4596")
}

// Migration scenario: both package-lock.json and yarn.lock present (e.g. a
// repo mid-migration between package managers). Node and yarn have distinct
// tool names, so both must be detected and cached independently — no
// collision, no dropped mount.
func TestDetectDirectoriesToCacheNodeAndYarnMigration(t *testing.T) {
	fl, err := os.Create(packageLockFile)
	test.Ok(t, err)
	defer fl.Close()
	_, err = fl.WriteString(testFileContent)
	test.Ok(t, err)

	fy, err := os.Create(yarnLockFile)
	test.Ok(t, err)
	defer fy.Close()
	_, err = fy.WriteString(testFileContent2)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(packageLockFile))
	test.Ok(t, os.RemoveAll(yarnLockFile))
	test.Ok(t, os.RemoveAll(".yarnrc"))
	test.Ok(t, os.RemoveAll(".yarnrc.yaml"))

	nodePath, _ := filepath.Abs("node_modules")
	yarnMigrationPath, _ := filepath.Abs(".yarn")
	expectedCacheDir := []string{nodePath, yarnMigrationPath}
	expectedDetectedTool := []string{toolNode, toolYarn}

	// DetectDirectoriesToCache concatenates each tool's own hex-encoded hash
	// (hashes += hash), not a single hash over combined raw bytes.
	expectedHash := md5Hex(testFileContent) + md5Hex(testFileContent2)

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, expectedHash)
}

// Polyglot repo: an unrelated build tool (maven) coexists with npm. The two
// must be detected independently with no cross-contamination of hashes or
// cache dirs.
func TestDetectDirectoriesToCacheNodeAndMavenPolyglot(t *testing.T) {
	fp, err := os.Create(pomFile)
	test.Ok(t, err)
	defer fp.Close()
	_, err = fp.WriteString(testFileContent)
	test.Ok(t, err)

	fl, err := os.Create(packageLockFile)
	test.Ok(t, err)
	defer fl.Close()
	_, err = fl.WriteString(testFileContent2)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(pomFile))
	test.Ok(t, os.RemoveAll(packageLockFile))

	mavenPath, _ := filepath.Abs(toolMavenDir)
	nodePolyglotPath, _ := filepath.Abs("node_modules")
	expectedCacheDir := []string{mavenPath, nodePolyglotPath}
	expectedDetectedTool := []string{toolMaven, toolNode}

	expectedHash := md5Hex(testFileContent) + md5Hex(testFileContent2)

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, expectedHash)
}

// npm workspaces: a root package-lock.json plus a nested one in a sub-package.
// The plain (non-recursive) glob matches the root file directly, so the
// nested one is never even considered — root must win, matching the "hoisted
// root node_modules" behavior workspaces rely on.
func TestDetectDirectoriesToCacheNodeWorkspaceRootWins(t *testing.T) {
	rootContent := testFileContent
	nestedContent := testFileContent2
	nestedDir := "sub-package"

	fRoot, err := os.Create(packageLockFile)
	test.Ok(t, err)
	defer fRoot.Close()
	_, err = fRoot.WriteString(rootContent)
	test.Ok(t, err)

	test.Ok(t, os.MkdirAll(nestedDir, 0755))
	fNested, err := os.Create(filepath.Join(nestedDir, packageLockFile))
	test.Ok(t, err)
	defer fNested.Close()
	_, err = fNested.WriteString(nestedContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(packageLockFile))
	test.Ok(t, os.RemoveAll(nestedDir))

	nodeWorkspacePath, _ := filepath.Abs("node_modules")
	expectedCacheDir := []string{nodeWorkspacePath}
	expectedDetectedTool := []string{toolNode}

	h := md5.New() // #nosec
	_, _ = h.Write([]byte(rootContent))
	expectedHash := hex.EncodeToString(h.Sum(nil))

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, expectedHash)
}

// A package-lock.json that only exists in a sub-directory (no root lockfile)
// must still be picked up via the recursive "**" fallback glob, caching
// node_modules at that sub-directory, not the repo root.
func TestDetectDirectoriesToCacheNodeNestedOnly(t *testing.T) {
	nestedDir := "nested-app"
	nestedContent := testFileContent

	test.Ok(t, os.MkdirAll(nestedDir, 0755))
	fNested, err := os.Create(filepath.Join(nestedDir, packageLockFile))
	test.Ok(t, err)
	defer fNested.Close()
	_, err = fNested.WriteString(nestedContent)
	test.Ok(t, err)

	directoriesToCache, buildToolsDetected, hashes, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Ok(t, os.RemoveAll(nestedDir))

	nodeNestedPath, _ := filepath.Abs(filepath.Join(nestedDir, "node_modules"))
	expectedCacheDir := []string{nodeNestedPath}
	expectedDetectedTool := []string{toolNode}

	h := md5.New() // #nosec
	_, _ = h.Write([]byte(nestedContent))
	expectedHash := hex.EncodeToString(h.Sum(nil))

	test.Equals(t, directoriesToCache, expectedCacheDir)
	test.Equals(t, buildToolsDetected, expectedDetectedTool)
	test.Equals(t, hashes, expectedHash)
}
