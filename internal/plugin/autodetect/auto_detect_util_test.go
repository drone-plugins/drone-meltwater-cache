package autodetect

import (
	"crypto/md5" // #nosec
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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

func writeRepoFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if dir := filepath.Dir(path); dir != "." {
			test.Ok(t, os.MkdirAll(dir, 0755))
		}
		test.Ok(t, os.WriteFile(path, []byte(content), 0644))
	}
}

func relCacheDirs(t *testing.T, wd string, dirs []string) []string {
	t.Helper()
	if dirs == nil {
		return nil
	}
	rel := make([]string, 0, len(dirs))
	for _, d := range dirs {
		r, err := filepath.Rel(wd, d)
		test.Ok(t, err)
		rel = append(rel, r)
	}
	return rel
}

func md5Concat(blobs ...string) string {
	h := md5.New() // #nosec
	for _, b := range blobs {
		_, _ = h.Write([]byte(b))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func clearNpmCacheEnv(t *testing.T) {
	t.Helper()
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
}

func TestDetectDirectoriesToCacheNode(t *testing.T) {
	clearNpmCacheEnv(t)

	const (
		lockA = "lock-content-a"
		lockB = "lock-content-b"
		pkgA  = `{"name":"app","version":"1.0.0"}`
	)

	tests := []struct {
		name         string
		files        map[string]string
		skipPrepare  bool
		wantTools    []string
		wantDirs     []string
		wantHash     string
		wantNoDetect bool
	}{
		{
			name:      "root package-lock.json caches node_modules and .npm",
			files:     map[string]string{packageLockFile: lockA},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name:      "package.json alone hashes package.json and still mounts npm dirs",
			files:     map[string]string{packageJSONFile: pkgA},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(pkgA),
		},
		{
			name: "package.json plus lockfile keys off the lockfile only",
			files: map[string]string{
				packageJSONFile: pkgA,
				packageLockFile: lockA,
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "empty package-lock.json is still a stable key",
			files: map[string]string{
				packageLockFile: "",
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(""),
		},
		{
			name: "yarn.lock without package-lock.json does not mount node_modules",
			files: map[string]string{
				packageJSONFile: pkgA,
				yarnLockFile:    lockA,
			},
			wantTools: []string{toolYarn},
			wantDirs:  []string{".yarn"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "npm and yarn lockfiles both present (migration)",
			files: map[string]string{
				packageLockFile: lockA,
				yarnLockFile:    lockB,
			},
			wantTools: []string{toolNode, toolYarn},
			wantDirs:  []string{"node_modules", ".npm", ".yarn"},
			wantHash:  md5Hex(lockA) + md5Hex(lockB),
		},
		{
			name: "polyglot maven plus npm",
			files: map[string]string{
				pomFile:         lockA,
				packageLockFile: lockB,
			},
			wantTools: []string{toolMaven, toolNode},
			wantDirs:  []string{toolMavenDir, "node_modules", ".npm"},
			wantHash:  md5Hex(lockA) + md5Hex(lockB),
		},
		{
			name: "npm workspaces: root lockfile wins over nested",
			files: map[string]string{
				packageLockFile: lockA,
				filepath.Join("packages", packageLockFile): lockB,
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "npm workspaces typical layout: root lockfile, packages two levels down",
			files: map[string]string{
				packageLockFile: lockA,
				filepath.Join("packages", "app", packageLockFile): lockB,
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "nested package.json without a lockfile is detected at one level",
			files: map[string]string{
				filepath.Join("nested-app", packageJSONFile): pkgA,
			},
			wantTools: []string{toolNode},
			wantDirs: []string{
				filepath.Join("nested-app", "node_modules"),
				filepath.Join("nested-app", ".npm"),
			},
			wantHash: md5Hex(pkgA),
		},
		{
			name: "mixed siblings: lockfile dir plus json-only dir",
			files: map[string]string{
				filepath.Join("backend", packageLockFile):  lockA,
				filepath.Join("frontend", packageJSONFile): pkgA,
			},
			wantTools: []string{toolNode},
			wantDirs: []string{
				filepath.Join("backend", "node_modules"),
				filepath.Join("backend", ".npm"),
				filepath.Join("frontend", "node_modules"),
				filepath.Join("frontend", ".npm"),
			},
			wantHash: md5Concat(lockA, pkgA),
		},
		{
			name: "package.json two directories down is outside glob depth",
			files: map[string]string{
				filepath.Join("apps", "web", packageJSONFile): pkgA,
			},
			wantNoDetect: true,
		},
		{
			name: "nested yarn.lock remains yarn when package.json is present",
			files: map[string]string{
				filepath.Join("app", packageJSONFile): pkgA,
				filepath.Join("app", yarnLockFile):    lockA,
			},
			wantTools: []string{toolYarn},
			wantDirs:  []string{filepath.Join("app", ".yarn")},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "nested lockfile one level down, no root",
			files: map[string]string{
				filepath.Join("nested-app", packageLockFile): lockA,
			},
			wantTools: []string{toolNode},
			wantDirs: []string{
				filepath.Join("nested-app", "node_modules"),
				filepath.Join("nested-app", ".npm"),
			},
			wantHash: md5Hex(lockA),
		},
		{
			name: "sibling apps without a root lockfile each get node_modules and .npm",
			files: map[string]string{
				filepath.Join("backend", packageLockFile):  lockA,
				filepath.Join("frontend", packageLockFile): lockB,
			},
			wantTools: []string{toolNode},
			wantDirs: []string{
				filepath.Join("backend", "node_modules"),
				filepath.Join("backend", ".npm"),
				filepath.Join("frontend", "node_modules"),
				filepath.Join("frontend", ".npm"),
			},
			wantHash: md5Concat(lockA, lockB),
		},
		{
			name: "three sibling packages, all cached",
			files: map[string]string{
				filepath.Join("a", packageLockFile): lockA,
				filepath.Join("b", packageLockFile): lockB,
				filepath.Join("c", packageLockFile): pkgA,
			},
			wantTools: []string{toolNode},
			wantDirs: []string{
				filepath.Join("a", "node_modules"),
				filepath.Join("a", ".npm"),
				filepath.Join("b", "node_modules"),
				filepath.Join("b", ".npm"),
				filepath.Join("c", "node_modules"),
				filepath.Join("c", ".npm"),
			},
			wantHash: md5Concat(lockA, lockB, pkgA),
		},
		{
			name: "existing .npmrc cache= is honored instead of default .npm",
			files: map[string]string{
				packageLockFile: lockA,
				npmrcFileName:   "registry=https://example.invalid\ncache=custom-npm\n",
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", "custom-npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "lockfile two directories down is outside glob depth",
			files: map[string]string{
				filepath.Join("apps", "web", packageLockFile): lockA,
			},
			wantNoDetect: true,
		},
		{
			name: "pnpm-lock.yaml is not npm and is not detected",
			files: map[string]string{
				packageJSONFile:  pkgA,
				"pnpm-lock.yaml": lockA,
			},
			wantNoDetect: true,
		},
		{
			name: "npm-shrinkwrap.json is hashed when package-lock.json is absent",
			files: map[string]string{
				"npm-shrinkwrap.json": lockA,
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "package-lock.json wins over npm-shrinkwrap.json",
			files: map[string]string{
				packageLockFile:       lockA,
				"npm-shrinkwrap.json": lockB,
			},
			wantTools: []string{toolNode},
			wantDirs:  []string{"node_modules", ".npm"},
			wantHash:  md5Hex(lockA),
		},
		{
			name: "bun.lock with package.json is not npm",
			files: map[string]string{
				packageJSONFile: pkgA,
				"bun.lock":      lockA,
			},
			wantNoDetect: true,
		},
		{
			name: "bun.lockb is not detected",
			files: map[string]string{
				"bun.lockb": lockA,
			},
			wantNoDetect: true,
		},
		{
			name: "skipPrepare still finds nothing to mount even with a lockfile",
			files: map[string]string{
				packageLockFile: lockA,
			},
			skipPrepare:  true,
			wantNoDetect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			wd, err := os.Getwd()
			test.Ok(t, err)

			writeRepoFiles(t, tc.files)

			dirs, tools, hashes, err := DetectDirectoriesToCache(tc.skipPrepare)
			test.Ok(t, err)

			if tc.wantNoDetect {
				test.Assert(t, dirs == nil, "expected no cache dirs, got %v", dirs)
				test.Assert(t, tools == nil, "expected no tools, got %v", tools)
				test.Equals(t, "", hashes)
				return
			}

			test.Equals(t, tc.wantDirs, relCacheDirs(t, wd, dirs))
			test.Equals(t, tc.wantTools, tools)
			test.Equals(t, tc.wantHash, hashes)
		})
	}
}

func TestDetectDirectoriesToCacheNodeLockfileHashIgnoresPackageJSON(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{
		packageJSONFile: `{"name":"app","version":"1.0.0"}`,
		packageLockFile: "stable-lock",
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Ok(t, os.WriteFile(packageJSONFile, []byte(`{"name":"app","version":"2.0.0"}`), 0644))

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Equals(t, hash1, hash2)
	test.Equals(t, md5Hex("stable-lock"), hash1)
}

func TestDetectDirectoriesToCacheNodeLockfileChangeInvalidatesHash(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{
		packageLockFile: "lock-v1",
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Ok(t, os.WriteFile(packageLockFile, []byte("lock-v2"), 0644))

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, hash1 != hash2, "lockfile change must invalidate cache key, got %s twice", hash1)
	test.Equals(t, md5Hex("lock-v1"), hash1)
	test.Equals(t, md5Hex("lock-v2"), hash2)
}

func TestDetectDirectoriesToCacheNodeHonorsNpmConfigCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", cacheDir)
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{packageLockFile: "lock"})

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	nodeModules, err := filepath.Abs("node_modules")
	test.Ok(t, err)
	cacheAbs, err := filepath.Abs(cacheDir)
	test.Ok(t, err)

	test.Equals(t, []string{toolNode}, tools)
	test.Equals(t, []string{nodeModules, cacheAbs}, dirs)

	_, statErr := os.Stat(npmrcFileName)
	test.Assert(t, os.IsNotExist(statErr), "npm_config_cache should not write .npmrc, got %v", statErr)
}

func TestDetectDirectoriesToCacheNodeHonorsUppercaseNpmConfigCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", cacheDir)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{packageLockFile: "lock"})

	dirs, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	nodeModules, err := filepath.Abs("node_modules")
	test.Ok(t, err)
	cacheAbs, err := filepath.Abs(cacheDir)
	test.Ok(t, err)
	test.Equals(t, []string{nodeModules, cacheAbs}, dirs)
}

func TestDetectDirectoriesToCacheNodeSharedTarballCacheForSiblings(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("npm_config_cache", cacheDir)
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{
		filepath.Join("backend", packageLockFile):  "backend-lock",
		filepath.Join("frontend", packageLockFile): "frontend-lock",
	})

	dirs, tools, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	backendNM, err := filepath.Abs(filepath.Join("backend", "node_modules"))
	test.Ok(t, err)
	frontendNM, err := filepath.Abs(filepath.Join("frontend", "node_modules"))
	test.Ok(t, err)
	cacheAbs, err := filepath.Abs(cacheDir)
	test.Ok(t, err)

	test.Equals(t, []string{toolNode}, tools)
	test.Equals(t, []string{backendNM, cacheAbs, frontendNM}, dirs)
}

func TestDetectDirectoriesToCacheNodeSkipPrepareDoesNotWriteNpmrc(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{packageLockFile: "lock"})

	_, _, _, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)

	_, statErr := os.Stat(npmrcFileName)
	test.Assert(t, os.IsNotExist(statErr), "skipPrepare must not write .npmrc, got %v", statErr)
}

func TestDetectDirectoriesToCacheNodePackageJSONChangeInvalidatesHash(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{
		packageJSONFile: `{"name":"app","version":"1.0.0"}`,
	})

	_, _, hash1, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Ok(t, os.WriteFile(packageJSONFile, []byte(`{"name":"app","version":"2.0.0"}`), 0644))

	_, _, hash2, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	test.Assert(t, hash1 != hash2, "package.json change must invalidate cache key")
	test.Equals(t, md5Hex(`{"name":"app","version":"1.0.0"}`), hash1)
}

func TestDetectDirectoriesToCacheNodeSaveReusesRestoreSidecarAfterLockfile(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	test.Ok(t, os.Mkdir(".git", 0755))
	writeRepoFiles(t, map[string]string{
		packageJSONFile: `{"name":"app","version":"1.0.0"}`,
	})

	dirs, tools, restoreHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{toolNode}, tools)
	test.Equals(t, md5Hex(`{"name":"app","version":"1.0.0"}`), restoreHash)
	test.Ok(t, WriteAutoKeySidecar(restoreHash))

	// npm install wrote a lockfile; Save must still use Restore's key.
	test.Ok(t, os.WriteFile(packageLockFile, []byte("generated-lock"), 0644))

	saveDirs, saveTools, saveHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{toolNode}, saveTools)
	test.Equals(t, md5Hex("generated-lock"), saveHash)
	test.Assert(t, restoreHash != saveHash, "redetect after lockfile must differ")

	sidecar, ok, err := ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "expected restore sidecar")
	test.Equals(t, restoreHash, sidecar)
	_, err = os.Stat(filepath.Join(".git", autoKeyFile))
	test.Ok(t, err)

	test.Equals(t, relCacheDirs(t, mustWd(t), dirs), relCacheDirs(t, mustWd(t), saveDirs))
}

func TestDetectDirectoriesToCacheNodeTrackedOnlyIgnoresUntrackedLockfile(t *testing.T) {
	requireGit(t)
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	gitInit(t)

	pkg := `{"name":"app","version":"1.0.0"}`
	writeRepoFiles(t, map[string]string{packageJSONFile: pkg})
	gitAdd(t, packageJSONFile)

	_, _, restoreHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, md5Hex(pkg), restoreHash)

	test.Ok(t, os.WriteFile(packageLockFile, []byte("generated-lock"), 0644))

	_, _, saveHash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, restoreHash, saveHash)
	test.Equals(t, md5Hex(pkg), saveHash)
}

func TestDetectDirectoriesToCacheNodeTrackedLockfileWins(t *testing.T) {
	requireGit(t)
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	gitInit(t)

	pkg := `{"name":"app","version":"1.0.0"}`
	writeRepoFiles(t, map[string]string{
		packageJSONFile: pkg,
		packageLockFile: "committed-lock",
	})
	gitAdd(t, packageJSONFile, packageLockFile)

	_, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{toolNode}, tools)
	test.Equals(t, md5Hex("committed-lock"), hash)
}

func TestDetectDirectoriesToCacheNodeTrackedJSONOnly(t *testing.T) {
	requireGit(t)
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	gitInit(t)

	pkg := `{"name":"app","version":"1.0.0"}`
	writeRepoFiles(t, map[string]string{packageJSONFile: pkg})
	gitAdd(t, packageJSONFile)

	_, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{toolNode}, tools)
	test.Equals(t, md5Hex(pkg), hash)
}

func TestDetectDirectoriesToCacheNodeTrackedMixedSiblings(t *testing.T) {
	requireGit(t)
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	gitInit(t)

	pkg := `{"name":"front","version":"1.0.0"}`
	writeRepoFiles(t, map[string]string{
		filepath.Join("backend", packageLockFile):  "lock-content-a",
		filepath.Join("frontend", packageJSONFile): pkg,
	})
	gitAdd(t, filepath.Join("backend", packageLockFile), filepath.Join("frontend", packageJSONFile))

	dirs, tools, hash, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{toolNode}, tools)
	test.Equals(t, []string{
		filepath.Join("backend", "node_modules"),
		filepath.Join("backend", ".npm"),
		filepath.Join("frontend", "node_modules"),
		filepath.Join("frontend", ".npm"),
	}, relCacheDirs(t, mustWd(t), dirs))
	test.Equals(t, md5Concat("lock-content-a", pkg), hash)
}

func TestDetectDirectoriesToCacheNodeSkipPrepareIgnoresPackageJSON(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{packageJSONFile: `{"name":"app"}`})

	dirs, tools, hashes, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Assert(t, dirs == nil, "expected no cache dirs, got %v", dirs)
	test.Assert(t, tools == nil, "expected no tools, got %v", tools)
	test.Equals(t, "", hashes)
}

func mustWd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	test.Ok(t, err)
	return wd
}

func TestDetectDirectoriesToCacheNodeWritesNpmrcForDefaultTarballCache(t *testing.T) {
	clearNpmCacheEnv(t)
	t.Chdir(t.TempDir())
	writeRepoFiles(t, map[string]string{packageLockFile: "lock"})

	_, _, _, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)

	data, err := os.ReadFile(npmrcFileName)
	test.Ok(t, err)
	wantCache, err := filepath.Abs(".npm")
	test.Ok(t, err)
	test.Assert(t, strings.Contains(string(data), "cache="+wantCache),
		"expected .npmrc to pin cache=%s, got %q", wantCache, data)
}
