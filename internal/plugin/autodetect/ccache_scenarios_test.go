package autodetect

import (
	"crypto/md5"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestScenario_Ccache_Detection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    []string
		selected int
	}{
		{"config", []string{"ccache.conf"}, 0},
		{"module", []string{"ccache.cmake"}, 0},
		{"nestedConfig", []string{"app/ccache.conf"}, 0},
		{"nestedModule", []string{"cmake/ccache.cmake"}, 0},
		{"deep", []string{"a/b/ccache.conf"}, -1},
		{"genericManifest", []string{"CMakeLists.txt"}, -1},
		{"precedence", []string{"ccache.cmake", "app/ccache.conf"}, 1},
		{"rootFirst", []string{"ccache.conf", "app/ccache.conf"}, 0},
		{"siblings", []string{"a/ccache.conf", "b/ccache.conf"}, 1},
		{"capitalized", []string{"cmake/Ccache.cmake"}, 0},
		{"camelCase", []string{"CMake/CCache.cmake"}, 0},
		{"prefixed", []string{"cmake/EnableCcache.cmake"}, 0},
		{"findModule", []string{"cmake/FindCCache.cmake"}, 0},
		{"underscored", []string{"CMakeModules/config_ccache.cmake"}, 0},
		{"exactBeforePrefixed", []string{"cmake/EnableCcache.cmake", "cmake/ccache.cmake"}, 1},
		{"sccache", []string{"cmake/sccache.cmake"}, -1},
		{"findSccache", []string{"cmake/Findsccache.cmake"}, -1},
		{"otherCmake", []string{"cmake/compiler.cmake"}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := ccacheWorkspace(t)
			for i, path := range tc.files {
				ccacheWrite(t, path, fmt.Sprintf("signal-%d", i))
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			if tc.selected < 0 {
				test.Equals(t, 0, len(tools))
				test.Equals(t, 0, len(dirs))
				test.Equals(t, "", key)
				return
			}
			test.Equals(t, []string{"ccache"}, tools)
			test.Equals(t, ccacheShards(filepath.Join(root, ".cache", "ccache")), dirs)
			test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("signal-%d", tc.selected)))), key)
		})
	}
}

func TestScenario_Ccache_Keys(t *testing.T) {
	ccacheWorkspace(t)
	ccacheWrite(t, "ccache.conf", "max_size = 5G\n")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("max_size = 5G\n"))), first)
	ccacheWrite(t, "src/file.c", "int main(void) { return 0; }")
	_, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, same)
	ccacheWrite(t, "ccache.conf", "max_size = 4G\n")
	_, _, markerChanged, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, markerChanged != first, "marker changes must invalidate")
}

// An npm lockfile belongs to node only: changing it must not reset the compiler cache.
func TestScenario_Ccache_KeyIgnoresNodeLock(t *testing.T) {
	ccacheWorkspace(t)
	ccacheWrite(t, "ccache.conf", "signal")
	marker := fmt.Sprintf("%x", md5.Sum([]byte("signal")))
	ccacheWrite(t, "package-lock.json", "lock-one")
	_, tools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, []string{"node", "ccache"}, tools)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("lock-one")))+marker, key)
	ccacheWrite(t, "package-lock.json", "lock-two")
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte("lock-two")))+marker, changed)
}

func TestScenario_Ccache_Mixed(t *testing.T) {
	root := ccacheWorkspace(t)
	t.Setenv("GOCACHE", filepath.Join(root, "go-cache"))
	ccacheWrite(t, "go.mod", "module example.com/test\n")
	dirs, tools, baseline, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	ccacheWrite(t, "ccache.conf", "signal")
	mixedDirs, mixedTools, key, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, append(tools, "ccache"), mixedTools)
	test.Equals(t, append(dirs, ccacheShards(filepath.Join(root, ".cache", "ccache"))...), mixedDirs)
	test.Equals(t, baseline+fmt.Sprintf("%x", md5.Sum([]byte("signal"))), key)
}

func TestScenario_Ccache_SkipPrepare(t *testing.T) {
	ccacheWorkspace(t)
	ccacheWrite(t, "ccache.conf", "signal")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
