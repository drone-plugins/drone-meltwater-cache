package autodetect

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func jsWorkspace(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	test.Ok(t, err)
	t.Chdir(root)
	isolateNpmEnv(t)
	return root
}

func Test_pnpmPreparer_StoreResolution(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pnpmEnv   string
		npmEnv    string
		workspace string
		npmrc     string
		stores    []string
	}{
		{name: "default", stores: []string{".cache/pnpm"}},
		{name: "pnpm11Env", pnpmEnv: "env-store", stores: []string{"env-store"}},
		{name: "pnpm10Env", npmEnv: "legacy-store", stores: []string{"legacy-store"}},
		{name: "bothEnvSame", pnpmEnv: "shared", npmEnv: "shared", stores: []string{"shared"}},
		{name: "workspaceFile", workspace: "storeDir: ws-store # pinned\n", stores: []string{"ws-store"}},
		{name: "quotedWorkspaceFile", workspace: "packages:\n  - 'pkgs/*'\nstoreDir: \"ws store\"\n", stores: []string{"ws store"}},
		{name: "npmrc", npmrc: "store-dir=npmrc-store\n", stores: []string{"npmrc-store"}},
		{
			name:      "allSources",
			pnpmEnv:   "env-store",
			npmEnv:    "legacy-store",
			workspace: "storeDir: ws-store\n",
			npmrc:     "store-dir=npmrc-store\n",
			stores:    []string{"env-store", "legacy-store", "ws-store", "npmrc-store"},
		},
		{name: "nestedKeyIgnored", workspace: "catalog:\n  storeDir: nope\n", stores: []string{".cache/pnpm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := jsWorkspace(t)
			t.Setenv("pnpm_config_store_dir", tc.pnpmEnv)
			t.Setenv("npm_config_store_dir", tc.npmEnv)
			if tc.workspace != "" {
				cargoWrite(t, pnpmWorkspaceFileName, tc.workspace)
			}
			if tc.npmrc != "" {
				cargoWrite(t, npmrcFileName, tc.npmrc)
			}

			primary, err := newPnpmPreparer().PrepareRepo(root)
			test.Ok(t, err)
			extra, err := pnpmCacheDirs(root)
			test.Ok(t, err)

			var expected []string
			for _, store := range tc.stores {
				expected = append(expected, filepath.Join(root, store))
			}
			test.Equals(t, expected[0], primary)
			test.Equals(t, append(expected[1:], filepath.Join(root, "node_modules")), extra)

			for _, store := range expected {
				_, err := os.Stat(store)
				test.Assert(t, os.IsNotExist(err), "preparer must not create the store")
			}
		})
	}
}

func Test_pnpmPreparer_HarnessWorkspaceDefault(t *testing.T) {
	root := jsWorkspace(t)
	t.Setenv("HARNESS_WORKSPACE", filepath.Join(root, "shared"))
	store, err := newPnpmPreparer().PrepareRepo(filepath.Join(root, "app"))
	test.Ok(t, err)
	test.Equals(t, filepath.Join(root, "shared", ".cache", "pnpm"), store)
}

func Test_pnpmPreparer_ConfigUntouched(t *testing.T) {
	root := jsWorkspace(t)
	files := map[string]string{
		pnpmWorkspaceFileName: "packages:\n  - 'pkgs/*'\n",
		npmrcFileName:         "auto-install-peers=true\n",
	}
	for name, content := range files {
		cargoWrite(t, name, content)
	}
	_, err := newPnpmPreparer().PrepareRepo(root)
	test.Ok(t, err)
	_, err = pnpmCacheDirs(root)
	test.Ok(t, err)
	for name, content := range files {
		got, err := os.ReadFile(name)
		test.Ok(t, err)
		test.Equals(t, content, string(got))
	}
	_, err = os.Stat(".pnpmfile.cjs")
	test.Assert(t, os.IsNotExist(err), "preparer must not create pnpm config")
}

func TestScenario_Pnpm_Detection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		dir   string
		tools []string
	}{
		{"root", []string{"package.json", "pnpm-lock.yaml"}, ".", []string{"pnpm"}},
		{"nested", []string{"app/package.json", "app/pnpm-lock.yaml"}, "app", []string{"pnpm"}},
		{"manifestOnly", []string{"package.json"}, ".", []string{"node"}},
		{"withPackageLock", []string{"package.json", "package-lock.json", "pnpm-lock.yaml"}, ".", []string{"node", "pnpm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := jsWorkspace(t)
			for i, f := range tc.files {
				cargoWrite(t, f, fmt.Sprintf("content-%d", i))
			}
			dirs, tools, _, err := DetectDirectoriesToCache(false)
			test.Ok(t, err)
			test.Equals(t, tc.tools, tools)
			if tc.tools[len(tc.tools)-1] != "pnpm" {
				return
			}
			test.Assert(t, containsTool(dirs, filepath.Join(root, ".cache", "pnpm")), "expected pnpm store in %v", dirs)
			test.Assert(t, containsTool(dirs, filepath.Join(root, tc.dir, "node_modules")), "expected node_modules in %v", dirs)
		})
	}
}

func TestScenario_Pnpm_Keys(t *testing.T) {
	jsWorkspace(t)
	cargoWrite(t, "package.json", "manifest")
	cargoWrite(t, "pnpm-lock.yaml", "first")
	_, _, first, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, fmt.Sprintf("%x%x", md5.Sum([]byte("manifest")), md5.Sum([]byte("first"))), first)

	cargoWrite(t, "pnpm-workspace.yaml", "packages:\n  - 'pkgs/*'\n")
	_, _, same, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, first, same)

	cargoWrite(t, "pnpm-lock.yaml", "second")
	_, _, changed, err := DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Assert(t, first != changed, "lockfile must invalidate the key")
}

func TestScenario_Pnpm_SkipPrepare(t *testing.T) {
	jsWorkspace(t)
	cargoWrite(t, "pnpm-lock.yaml", "lock")
	dirs, tools, key, err := DetectDirectoriesToCache(true)
	test.Ok(t, err)
	test.Equals(t, 0, len(dirs))
	test.Equals(t, 0, len(tools))
	test.Equals(t, "", key)
}
