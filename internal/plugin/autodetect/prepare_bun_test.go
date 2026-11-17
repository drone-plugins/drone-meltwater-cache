package autodetect

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func Test_bunPreparer_CacheResolution(t *testing.T) {
	for _, tc := range []struct {
		name, env, install, bunfig, cache string
	}{
		{name: "default", cache: ".cache/bun"},
		{name: "env", env: "env-cache", cache: "env-cache"},
		{name: "envSpaces", env: " bun cache ", cache: " bun cache "},
		{name: "bunfig", bunfig: "[install.cache]\ndir = \"bunfig-cache\" # pinned\n", cache: "bunfig-cache"},
		{name: "envBeatsBunfig", env: "env-cache", bunfig: "[install.cache]\ndir = \"bunfig-cache\"\n", cache: "env-cache"},
		{name: "otherTableIgnored", bunfig: "[install]\ndir = \"nope\"\n", cache: ".cache/bun"},
		{name: "bunInstall", install: "bun-home", cache: "bun-home/install/cache"},
		{name: "bunfigBeatsBunInstall", install: "bun-home", bunfig: "[install.cache]\ndir = 'bunfig-cache'\n", cache: "bunfig-cache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := jsWorkspace(t)
			t.Setenv("BUN_INSTALL_CACHE_DIR", tc.env)
			t.Setenv("BUN_INSTALL", tc.install)
			if tc.bunfig != "" {
				cargoWrite(t, "bunfig.toml", tc.bunfig)
			}

			cache, err := newBunPreparer().PrepareRepo(root)
			test.Ok(t, err)
			expected := filepath.Join(root, tc.cache)
			if tc.env != "" {
				expected = filepath.Join(root, tc.env)
			}
			test.Equals(t, expected, cache)
			_, err = os.Stat(cache)
			test.Assert(t, os.IsNotExist(err), "preparer must not create the cache")

			if tc.bunfig != "" {
				got, err := os.ReadFile("bunfig.toml")
				test.Ok(t, err)
				test.Equals(t, tc.bunfig, string(got))
			}
		})
	}
}

func TestScenario_Bun_Detection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		dir   string
		key   string
	}{
		{"textLock", []string{"bun.lock"}, ".", "content-0"},
		{"binaryLock", []string{"bun.lockb"}, ".", "content-0"},
		{"textWinsOverBinary", []string{"bun.lockb", "bun.lock"}, ".", "content-1"},
		{"nested", []string{"app/bun.lock"}, "app", "content-0"},
		{"nestedBinary", []string{"app/bun.lockb"}, "app", "content-0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := jsWorkspace(t)
			for i, f := range tc.files {
				cargoWrite(t, f, fmt.Sprintf("content-%d", i))
			}
			dirs, tools, key, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			test.Equals(t, []string{"bun"}, tools)
			test.Equals(t, fmt.Sprintf("%x", md5.Sum([]byte(tc.key))), key)
			test.Equals(t, []string{filepath.Join(root, ".cache", "bun"), filepath.Join(root, tc.dir, "node_modules")}, dirs)
		})
	}
}

func TestScenario_Bun_Keys(t *testing.T) {
	jsWorkspace(t)
	cargoWrite(t, "package.json", "manifest")
	cargoWrite(t, "bun.lock", "first")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, fmt.Sprintf("%x%x", md5.Sum([]byte("manifest")), md5.Sum([]byte("first"))), first)

	cargoWrite(t, "bun.lock", "second")
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, first != changed, "lockfile must invalidate the key")

	cargoWrite(t, "package.json", "manifest-2")
	_, _, manifestChanged, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, changed != manifestChanged, "package.json must invalidate the key")
}
