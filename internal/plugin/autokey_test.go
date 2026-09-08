package plugin

import (
	"os"
	"path/filepath"
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

func TestExecPackageJSONFallbackSurvivesGeneratedLockfile(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "execution-1")

	packageJSON := `{"name":"app","version":"1.0.0"}`
	test.Ok(t, os.WriteFile("package.json", []byte(packageJSON), 0644))

	cacheRoot := t.TempDir()

	// Build 1 restore is cold, but records the package.json decision.
	_ = autoDetectPlugin(t, cacheRoot, true, false).Exec()
	plan, found, err := autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, found, "restore should record a package.json fallback plan")

	// npm install generates a lockfile and node_modules after Restore.
	test.Ok(t, os.WriteFile("package-lock.json", []byte("generated-lock"), 0644))
	test.Ok(t, os.MkdirAll("node_modules", 0755))
	test.Ok(t, os.WriteFile(filepath.Join("node_modules", "index.js"), []byte("cached"), 0644))

	test.Ok(t, autoDetectPlugin(t, cacheRoot, false, true).Exec())

	_, found, err = autodetect.ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "successful save should consume the fallback plan")
	_, err = os.Stat(".npmrc")
	test.Assert(t, os.IsNotExist(err), "fallback save must not prepare the npm tarball cache")
	_, err = os.Stat(".npm")
	test.Assert(t, os.IsNotExist(err), "fallback save must only cache node_modules")

	// Build 2 starts from the same clean package.json-only checkout.
	test.Ok(t, os.Remove("package-lock.json"))
	test.Ok(t, os.RemoveAll("node_modules"))
	setPluginPlanScope(t, "execution-2")

	test.Ok(t, autoDetectPlugin(t, cacheRoot, true, false).Exec())
	restored, err := os.ReadFile(filepath.Join("node_modules", "index.js"))
	test.Ok(t, err)
	test.Equals(t, "cached", string(restored))
	test.Assert(t, plan.Key != "", "expected a package.json-derived cache key")
}

func TestExecLockfileSaveDoesNotRequireFallbackPlan(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "execution")

	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))
	test.Ok(t, os.WriteFile("package-lock.json", []byte("lock"), 0644))
	test.Ok(t, os.MkdirAll("node_modules", 0755))
	test.Ok(t, os.WriteFile(filepath.Join("node_modules", "index.js"), []byte("cached"), 0644))

	test.Ok(t, autoDetectPlugin(t, t.TempDir(), false, true).Exec())
}

func setPluginPlanScope(t *testing.T, execution string) {
	t.Helper()
	t.Setenv("HARNESS_EXECUTION_ID", execution)
	t.Setenv("HARNESS_STAGE_ID", "stage")
	t.Setenv("HARNESS_ACCOUNT_ID", "account")
	t.Setenv("HARNESS_ORG_ID", "org")
	t.Setenv("HARNESS_PROJECT_ID", "project")
	t.Setenv("HARNESS_PIPELINE_ID", "pipeline")
}

func autoDetectPlugin(t *testing.T, cacheRoot string, restore, rebuild bool) *Plugin {
	t.Helper()

	return &Plugin{
		logger: log.NewNopLogger(),
		Metadata: metadata.Metadata{
			Repo:   metadata.Repo{Branch: "master", Name: "drone-cache"},
			Commit: metadata.Commit{Branch: "master"},
		},
		Config: Config{
			ArchiveFormat:           archive.Tar,
			Backend:                 backend.FileSystem,
			AccountID:               "account",
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
