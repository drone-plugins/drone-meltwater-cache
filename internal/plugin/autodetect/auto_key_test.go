package autodetect

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func setAutoPlanEnv(t *testing.T, execution, stage string) {
	t.Helper()
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	t.Setenv("HARNESS_EXECUTION_ID", execution)
	t.Setenv("HARNESS_STAGE_ID", stage)
	t.Setenv("HARNESS_ACCOUNT_ID", "account")
	t.Setenv("HARNESS_ORG_ID", "org")
	t.Setenv("HARNESS_PROJECT_ID", "project")
	t.Setenv("HARNESS_PIPELINE_ID", "pipeline")
}

func TestAutoDetectPlanRoundTripAndPathFilter(t *testing.T) {
	setAutoPlanEnv(t, "execution", "stage")

	workspace := t.TempDir()
	nodeModules := filepath.Join(workspace, "node_modules")
	npmCache := filepath.Join(workspace, ".npm")
	plan := AutoDetectPlan{
		Key:     md5Hex(t, "package"),
		PathIDs: PathIdentities([]string{nodeModules}),
	}
	test.Ok(t, WriteAutoDetectPlan(plan))

	stored, found, err := ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, found, "expected plan")
	test.Equals(t, stored.Key, plan.Key)
	test.Equals(t, FilterPathsForPlan([]string{nodeModules, npmCache}, stored.PathIDs), []string{nodeModules})

	test.Ok(t, RemoveAutoDetectPlan())
	_, found, err = ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "expected plan to be removed")
}

func TestAutoDetectPlanIsScopedToExecution(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HARNESS_TMP_PATH", tmp)
	t.Setenv("HARNESS_STAGE_ID", "stage")
	t.Setenv("HARNESS_EXECUTION_ID", "first")

	plan := AutoDetectPlan{
		Key:     md5Hex(t, "package"),
		PathIDs: PathIdentities([]string{filepath.Join(tmp, "node_modules")}),
	}
	test.Ok(t, WriteAutoDetectPlan(plan))

	t.Setenv("HARNESS_EXECUTION_ID", "second")
	_, found, err := ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "another execution must not see this plan")
}

func TestAutoDetectPlanRejectsSymlink(t *testing.T) {
	setAutoPlanEnv(t, "execution", "stage")

	path, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Ok(t, os.MkdirAll(filepath.Dir(path), 0755))
	target := filepath.Join(t.TempDir(), "target")
	test.Ok(t, os.WriteFile(target, []byte("{}"), 0600))
	test.Ok(t, os.Symlink(target, path))

	_, _, err = ReadAutoDetectPlan()
	test.Assert(t, errors.Is(err, ErrInvalidPlan), "expected invalid plan, got %v", err)
}

func TestAutoDetectPlanRequiresHarnessTmpPath(t *testing.T) {
	t.Setenv("HARNESS_TMP_PATH", "")
	t.Setenv("HARNESS_EXECUTION_ID", "execution")
	t.Setenv("HARNESS_STAGE_ID", "stage")

	err := WriteAutoDetectPlan(AutoDetectPlan{
		Key:     md5Hex(t, "package"),
		PathIDs: PathIdentities([]string{"/workspace/node_modules"}),
	})
	test.Assert(t, errors.Is(err, ErrInvalidPlan), "expected invalid plan root, got %v", err)
}
