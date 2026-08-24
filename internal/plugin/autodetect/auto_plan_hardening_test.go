package autodetect

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func setupPlanTest(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	t.Setenv(harnessTmpPathEnv, "")
	t.Setenv(harnessScratchDirEnv, "")
	t.Setenv(harnessWorkspaceEnv, workspace)
	setPlanScope(t, "execution", "stage", "0")
	return workspace
}

func TestAutoDetectPlanPrefersTmpPath(t *testing.T) {
	setPlanScope(t, "execution", "stage", "0")
	tmpPath := t.TempDir()
	scratch := t.TempDir()
	workspace := t.TempDir()
	t.Setenv(harnessTmpPathEnv, tmpPath)
	t.Setenv(harnessScratchDirEnv, scratch)
	t.Setenv(harnessWorkspaceEnv, workspace)

	path, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Equals(t, filepath.Join(tmpPath, autoPlanDirName), filepath.Dir(path))
}

func TestAutoDetectPlanFallsBackToScratchDirectory(t *testing.T) {
	setPlanScope(t, "execution", "stage", "0")
	scratch := t.TempDir()
	workspace := t.TempDir()
	t.Setenv(harnessTmpPathEnv, "")
	t.Setenv(harnessScratchDirEnv, scratch)
	t.Setenv(harnessWorkspaceEnv, workspace)

	path, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Equals(t, filepath.Join(scratch, autoPlanDirName), filepath.Dir(path))
}

func TestAutoDetectPlanFallsBackToWorkspace(t *testing.T) {
	setPlanScope(t, "execution", "stage", "0")
	workspace := t.TempDir()
	t.Setenv(harnessTmpPathEnv, "")
	t.Setenv(harnessScratchDirEnv, "")
	t.Setenv(harnessWorkspaceEnv, workspace)

	path, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Equals(t, filepath.Join(workspace, autoPlanDirName), filepath.Dir(path))
}

func TestAutoDetectPlanWithoutSharedRootIsDisabled(t *testing.T) {
	setPlanScope(t, "execution", "stage", "0")
	t.Setenv(harnessTmpPathEnv, "")
	t.Setenv(harnessScratchDirEnv, "")
	t.Setenv(harnessWorkspaceEnv, "")

	_, _, err := autoPlanPath()
	test.Assert(t, errors.Is(err, ErrInvalidPlan), "expected invalid plan root, got %v", err)
	test.Assert(t, strings.Contains(err.Error(), harnessTmpPathEnv), "error should name the tmp env: %v", err)
	test.Assert(t, strings.Contains(err.Error(), harnessScratchDirEnv), "error should name the scratch env: %v", err)
	test.Assert(t, strings.Contains(err.Error(), harnessWorkspaceEnv), "error should name the workspace env: %v", err)
}

func TestAutoDetectPlanRestoreAndSaveUseSameTmpPath(t *testing.T) {
	setPlanScope(t, "execution", "stage", "0")
	tmpPath := t.TempDir()
	t.Setenv(harnessTmpPathEnv, tmpPath)
	t.Setenv(harnessScratchDirEnv, t.TempDir())
	t.Setenv(harnessWorkspaceEnv, t.TempDir())

	restorePath, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Ok(t, WriteAutoDetectPlan(AutoDetectPlan{Key: "restore-key"}))

	savePath, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Equals(t, restorePath, savePath)
	plan, found, err := ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, found, "save should find the restore plan in the same temp directory")
	test.Equals(t, "restore-key", plan.Key)
}

func TestAutoDetectPlanDoesNotDirtyProjectGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	project := t.TempDir()
	cmd := exec.Command("git", "init", project)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(project)

	setPlanScope(t, "execution", "stage", "0")
	tmpPath := t.TempDir()
	t.Setenv(harnessTmpPathEnv, tmpPath)
	t.Setenv(harnessScratchDirEnv, "")
	t.Setenv(harnessWorkspaceEnv, "")
	test.Ok(t, WriteAutoDetectPlan(AutoDetectPlan{Key: "key"}))

	status := exec.Command("git", "status", "--porcelain")
	status.Dir = project
	out, err := status.Output()
	test.Ok(t, err)
	test.Equals(t, "", strings.TrimSpace(string(out)))
}

func TestAutoDetectPlanExecutionStageAndMatrixScopesUseDifferentFiles(t *testing.T) {
	setupPlanTest(t)
	baseline, _, err := autoPlanPath()
	test.Ok(t, err)

	cases := []struct {
		name      string
		execution string
		stage     string
		matrix    string
	}{
		{name: "execution", execution: "other-execution", stage: "stage", matrix: "0"},
		{name: "stage", execution: "execution", stage: "other-stage", matrix: "0"},
		{name: "matrix", execution: "execution", stage: "stage", matrix: "1"},
	}
	seen := map[string]bool{baseline: true}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setPlanScope(t, tc.execution, tc.stage, tc.matrix)
			path, _, err := autoPlanPath()
			test.Ok(t, err)
			test.Assert(t, !seen[path], "%s scope must use a distinct plan file", tc.name)
			seen[path] = true
		})
	}
}

func TestAutoDetectPlanPermissionsAndOverwrite(t *testing.T) {
	setupPlanTest(t)
	test.Ok(t, WriteAutoDetectPlan(AutoDetectPlan{Key: "first"}))
	path, _, err := autoPlanPath()
	test.Ok(t, err)
	test.Ok(t, os.Chmod(path, 0o666))
	test.Ok(t, WriteAutoDetectPlan(AutoDetectPlan{Key: "second"}))

	fileInfo, err := os.Stat(path)
	test.Ok(t, err)
	test.Equals(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(path))
	test.Ok(t, err)
	test.Equals(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	plan, found, err := ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, found, "overwritten plan should be readable")
	test.Equals(t, "second", plan.Key)
}

func TestAutoDetectPlanRejectsCorruptOversizedAndUnknownFields(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "partial json", data: []byte(`{"key":"partial"`)},
		{name: "oversized", data: []byte(strings.Repeat("x", autoPlanMaxSize+1))},
		{name: "persisted paths", data: []byte(`{"key":"key","scope":"scope","paths":["/var/run/secrets"]}`)},
		{name: "trailing json", data: []byte(`{"key":"key","scope":"scope"} {}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupPlanTest(t)
			path, _, err := autoPlanPath()
			test.Ok(t, err)
			test.Ok(t, securePlanDir(path))
			test.Ok(t, os.WriteFile(path, tc.data, 0o600))

			_, found, err := ReadAutoDetectPlan()
			test.Assert(t, errors.Is(err, ErrInvalidPlan), "expected invalid plan, got %v", err)
			test.Assert(t, !found, "invalid plan must not be accepted")
		})
	}
}

func TestAutoDetectPlanRejectsSymlinkAndNonRegularFile(t *testing.T) {
	tests := []struct {
		name  string
		setup func(string) error
	}{
		{
			name: "symlink",
			setup: func(path string) error {
				target := filepath.Join(filepath.Dir(path), "target")
				if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
					return err
				}
				return os.Symlink(target, path)
			},
		},
		{
			name:  "directory",
			setup: func(path string) error { return os.Mkdir(path, 0o700) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupPlanTest(t)
			path, _, err := autoPlanPath()
			test.Ok(t, err)
			test.Ok(t, securePlanDir(path))
			test.Ok(t, tc.setup(path))

			_, found, err := ReadAutoDetectPlan()
			test.Assert(t, errors.Is(err, ErrInvalidPlan), "expected invalid plan path, got %v", err)
			test.Assert(t, !found, "non-regular plan must not be accepted")
			err = WriteAutoDetectPlan(AutoDetectPlan{Key: "key"})
			test.Assert(t, errors.Is(err, ErrInvalidPlan), "write must not follow or replace non-regular path")
		})
	}
}

func TestAutoDetectPlanScopeMismatchIsRemoved(t *testing.T) {
	setupPlanTest(t)
	path, scope, err := autoPlanPath()
	test.Ok(t, err)
	test.Ok(t, securePlanDir(path))
	data, err := json.Marshal(AutoDetectPlan{Key: "key", Scope: scope + "-other"})
	test.Ok(t, err)
	test.Ok(t, os.WriteFile(path, data, 0o600))

	_, found, err := ReadAutoDetectPlan()
	test.Assert(t, errors.Is(err, ErrPlanScopeMismatch), "expected scope mismatch, got %v", err)
	test.Assert(t, !found, "mismatched plan must not be returned")
	_, statErr := os.Stat(path)
	test.Assert(t, os.IsNotExist(statErr), "mismatched plan should be removed")
}

func TestAutoDetectPlanCleanup(t *testing.T) {
	setupPlanTest(t)
	test.Ok(t, WriteAutoDetectPlan(AutoDetectPlan{Key: "key"}))
	test.Ok(t, RemoveAutoDetectPlan())
	_, found, err := ReadAutoDetectPlan()
	test.Ok(t, err)
	test.Assert(t, !found, "removed plan must not remain readable")
}

func TestFilterPathsForPackageJSONSource(t *testing.T) {
	project := t.TempDir()
	sum := sourceIdentity(project)
	npmCache := filepath.Join(project, ".npm")
	nodeModules := filepath.Join(project, "node_modules")
	otherNodeModules := filepath.Join(t.TempDir(), "node_modules")
	got := FilterPathsForPlan([]string{nodeModules, npmCache, otherNodeModules}, []string{sum})
	test.Equals(t, []string{npmCache, otherNodeModules}, got)
}

func TestValidateDetectedPathsRejectsMalformedAndSymlinkPaths(t *testing.T) {
	test.NotOk(t, ValidateDetectedPaths([]string{"../secrets"}))
	test.NotOk(t, ValidateDetectedPaths([]string{string(os.PathSeparator) + "tmp/../secrets"}))
	tooMany := make([]string, maxDetectedPaths+1)
	test.NotOk(t, ValidateDetectedPaths(tooMany))

	root := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(root, "cache-link")
	test.Ok(t, os.Symlink(target, link))
	t.Chdir(root)
	test.NotOk(t, ValidateDetectedPaths([]string{filepath.Join(link, "contents")}))
}

func sourceIdentity(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return fmt.Sprintf("%x", sum[:])
}
