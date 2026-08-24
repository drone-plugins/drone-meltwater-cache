package plugin

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"

	"github.com/meltwater/drone-cache/archive"
	"github.com/meltwater/drone-cache/internal/metadata"
	"github.com/meltwater/drone-cache/internal/plugin/autodetect"
	"github.com/meltwater/drone-cache/storage/backend"
	"github.com/meltwater/drone-cache/storage/backend/filesystem"
	"github.com/meltwater/drone-cache/test"
)

func TestAutoCacheKey(t *testing.T) {
	test.Equals(t, "acct/manifest-hash", autoCacheKey("acct", "manifest-hash"))
}

// A project with no committed lockfile is the case the recorded plan exists for.
// The restore step keys off package.json; npm install then writes a lockfile, so
// a save step running detection itself would pick a different key and a
// different path set, and the remote object name embeds both. This walks two
// consecutive builds and asserts the second one actually hits.
func TestExecSaveReplaysRestorePlanAcrossBuilds(t *testing.T) {
	npmCache := t.TempDir()
	t.Setenv("npm_config_cache", npmCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())
	t.Chdir(t.TempDir())

	pkg := `{"name":"app","version":"1.0.0"}`
	test.Ok(t, os.WriteFile("package.json", []byte(pkg), 0644))

	cacheRoot := t.TempDir()

	// Build 1, restore: cold cache, so the fetch misses. The plan is still
	// recorded, which is the part that matters here.
	_ = autoDetectPlugin(t, cacheRoot, true, false, "").Exec()

	plan, found, err := autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, found, "the restore step should record a plan")
	test.Assert(t, len(plan.Sources) == 1, "package.json-only restore should record one source identity")

	// npm install: resolves the tree, writes a lockfile, fills the tarball cache.
	test.Ok(t, os.WriteFile("package-lock.json", []byte("generated-lock"), 0644))
	test.Ok(t, os.MkdirAll(filepath.Join(npmCache, "_cacache"), 0755))
	test.Ok(t, os.WriteFile(filepath.Join(npmCache, "_cacache", "index"), []byte("tarballs"), 0644))

	// Build 1, save.
	test.Ok(t, autoDetectPlugin(t, cacheRoot, false, true, "").Exec())

	objects := cacheObjects(t, cacheRoot)
	test.Assert(t, len(objects) > 0, "the save step should write cache objects")
	for _, obj := range objects {
		test.Assert(t, strings.Contains(obj, plan.Key),
			"object %s should be stored under the key the restore step resolved (%s)", obj, plan.Key)
		test.Assert(t, !strings.Contains(obj, "node_modules"),
			"node_modules must not be cached without a lockfile, got %s", obj)
	}
	_, found, err = autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "a successful save must consume its plan")

	// Build 2: a fresh checkout has package.json but not the generated lockfile,
	// a fresh temp path, and an empty tarball cache.
	test.Ok(t, os.Remove("package-lock.json"))
	test.Ok(t, os.RemoveAll(npmCache))
	test.Ok(t, os.MkdirAll(npmCache, 0755))
	setPluginPlanScope(t, "execution-2", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())

	test.Ok(t, autoDetectPlugin(t, cacheRoot, true, false, "").Exec())

	restored, err := os.ReadFile(filepath.Join(npmCache, "_cacache", "index"))
	test.Ok(t, err)
	test.Equals(t, "tarballs", string(restored))
}

func TestExecMissingPlanSkipsSave(t *testing.T) {
	npmCache := t.TempDir()
	t.Setenv("npm_config_cache", npmCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())
	t.Chdir(t.TempDir())

	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))
	test.Ok(t, os.WriteFile("package-lock.json", []byte("committed-lock"), 0644))
	test.Ok(t, os.MkdirAll("node_modules", 0755))
	test.Ok(t, os.WriteFile(filepath.Join("node_modules", "index.js"), []byte("ok"), 0644))
	test.Ok(t, os.MkdirAll(filepath.Join(npmCache, "_cacache"), 0755))
	test.Ok(t, os.WriteFile(filepath.Join(npmCache, "_cacache", "index"), []byte("tarballs"), 0644))

	cacheRoot := t.TempDir()
	test.Ok(t, autoDetectPlugin(t, cacheRoot, false, true, "").Exec())

	test.Equals(t, 0, len(cacheObjects(t, cacheRoot)))
}

func TestExecPackageJSONOnlyWithEmptyTarballCacheCompletes(t *testing.T) {
	t.Setenv("npm_config_cache", t.TempDir())
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())
	t.Chdir(t.TempDir())
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))

	cacheRoot := t.TempDir()
	_ = autoDetectPlugin(t, cacheRoot, true, false, "").Exec() // cold cache miss
	test.Ok(t, autoDetectPlugin(t, cacheRoot, false, true, "").Exec())
	test.Equals(t, 1, len(cacheObjects(t, cacheRoot)))

	_, found, err := autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "a successful save must consume the restore plan")
	_, statErr := os.Stat(".npmrc")
	test.Assert(t, os.IsNotExist(statErr), ".npmrc must not be created")
}

func TestExecCorruptPlanSkipsSaveAndCleansPlan(t *testing.T) {
	npmCache := t.TempDir()
	t.Setenv("npm_config_cache", npmCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	workspace := t.TempDir()
	t.Setenv("HARNESS_WORKSPACE", workspace)
	t.Chdir(t.TempDir())
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))

	cacheRoot := t.TempDir()
	_ = autoDetectPlugin(t, cacheRoot, true, false, "").Exec()
	plans, err := filepath.Glob(filepath.Join(workspace, ".cache-intelligence", "*.json"))
	test.Ok(t, err)
	test.Equals(t, 1, len(plans))
	test.Ok(t, os.WriteFile(plans[0], []byte(`{"key":`), 0600))

	test.Ok(t, autoDetectPlugin(t, cacheRoot, false, true, "").Exec())
	test.Equals(t, 0, len(cacheObjects(t, cacheRoot)))
	_, err = os.Stat(plans[0])
	test.Assert(t, os.IsNotExist(err), "corrupt plan should be removed safely")
}

func TestExecFailedSaveRetainsPlanForRetry(t *testing.T) {
	t.Setenv("npm_config_cache", t.TempDir())
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())
	t.Chdir(t.TempDir())
	test.Ok(t, os.WriteFile("package-lock.json", []byte("lock"), 0644))
	test.Ok(t, os.MkdirAll("node_modules", 0755))
	test.Ok(t, os.WriteFile(filepath.Join("node_modules", "index.js"), []byte("ok"), 0644))

	_ = autoDetectPlugin(t, t.TempDir(), true, false, "").Exec()
	badCacheRoot := filepath.Join(t.TempDir(), "not-a-directory")
	test.Ok(t, os.WriteFile(badCacheRoot, []byte("x"), 0600))
	err := autoDetectPlugin(t, badCacheRoot, false, true, "").Exec()
	test.NotOk(t, err)

	_, found, readErr := autodetect.ReadAutoDetectPlan()
	test.Ok(t, readErr)
	test.Assert(t, found, "failed save must retain the plan for an in-execution retry")
}

func TestExecCustomCacheKeyDoesNotRecordPlan(t *testing.T) {
	t.Setenv("npm_config_cache", t.TempDir())
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())
	t.Chdir(t.TempDir())

	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))

	_ = autoDetectPlugin(t, t.TempDir(), true, false, "user-key").Exec()

	_, found, err := autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "a custom cache key must not record an autodetection plan")
}

func TestExecCustomMountPathDoesNotRecordPlan(t *testing.T) {
	t.Setenv("npm_config_cache", t.TempDir())
	t.Setenv("NPM_CONFIG_CACHE", "")
	setPluginPlanScope(t, "execution-1", "build", "0")
	t.Setenv("HARNESS_WORKSPACE", t.TempDir())
	t.Chdir(t.TempDir())

	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))

	mounted := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(mounted, "keep"), []byte("x"), 0644))

	p := autoDetectPlugin(t, t.TempDir(), true, false, "")
	p.Config.Mount = []string{mounted}
	_ = p.Exec()

	_, found, err := autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "a custom mount path must not record an autodetection plan")
	test.Equals(t, 0, len(cacheObjects(t, p.Config.FileSystem.CacheRoot)))
}

func setPluginPlanScope(t *testing.T, execution, stage, matrix string) {
	t.Helper()
	t.Setenv("HARNESS_TMP_PATH", "")
	t.Setenv("HARNESS_SCRATCH_DIR", "")
	t.Setenv("HARNESS_EXECUTION_ID", execution)
	t.Setenv("HARNESS_STAGE_ID", stage)
	t.Setenv("HARNESS_PIPELINE_ID", "pipeline")
	t.Setenv("DRONE_REPO", "org/repo")
	t.Setenv("HARNESS_STAGE_INDEX", matrix)
}

func cacheObjects(t *testing.T, root string) []string {
	t.Helper()

	var out []string

	test.Ok(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		out = append(out, filepath.ToSlash(rel))

		return nil
	}))

	sort.Strings(out)

	return out
}

func autoDetectPlugin(t *testing.T, cacheRoot string, restore, rebuild bool, key string) *Plugin {
	t.Helper()

	return &Plugin{
		logger: log.NewNopLogger(),
		Metadata: metadata.Metadata{
			Repo: metadata.Repo{
				Branch: "master",
				Name:   "drone-cache",
			},
			Commit: metadata.Commit{Branch: "master"},
		},
		Config: Config{
			ArchiveFormat:           archive.Tar,
			Backend:                 backend.FileSystem,
			CacheKeyTemplate:        key,
			AccountID:               "acct",
			Rebuild:                 rebuild,
			Restore:                 restore,
			AutoDetect:              true,
			Override:                true,
			CompressionLevel:        archive.DefaultCompressionLevel,
			StorageOperationTimeout: 5 * time.Second,
			FileSystem:              filesystem.Config{CacheRoot: cacheRoot},
		},
	}
}
