package autodetect

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	harnessTmpPathEnv    = "HARNESS_TMP_PATH"
	harnessScratchDirEnv = "HARNESS_SCRATCH_DIR"
	harnessWorkspaceEnv  = "HARNESS_WORKSPACE"
	autoPlanDirName      = ".cache-intelligence"
	autoPlanMaxSize      = 64 * 1024
	autoPlanMaxKeySize   = 4096
	autoPlanMaxSources   = 256
)

var (
	ErrInvalidPlanScope  = errors.New("cache plan scope is incomplete")
	ErrInvalidPlan       = errors.New("invalid cache plan")
	ErrPlanScopeMismatch = errors.New("cache plan scope does not match this execution")
)

// AutoDetectPlan carries only the key chosen by Restore. Save detects paths
// again, so a build step cannot inject arbitrary archive paths through this file.
type AutoDetectPlan struct {
	Key     string   `json:"key"`
	Scope   string   `json:"scope"`
	Sources []string `json:"sources,omitempty"`
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

// planScope uses identifiers injected into both generated cache steps. Matrix
// indexes separate parallel stage executions that otherwise share identifiers.
func planScope() (string, error) {
	execution := firstEnv("HARNESS_EXECUTION_ID", "HARNESS_BUILD_ID", "DRONE_BUILD_NUMBER")
	stage := firstEnv("HARNESS_STAGE_ID", "DRONE_STAGE_NUMBER", "DRONE_STAGE_NAME")
	repository := firstEnv("DRONE_REPO")
	if repository == "" {
		namespace := firstEnv("DRONE_REPO_NAMESPACE", "DRONE_REPO_OWNER")
		name := firstEnv("DRONE_REPO_NAME")
		if namespace != "" && name != "" {
			repository = namespace + "/" + name
		}
	}
	pipeline := firstEnv("HARNESS_PIPELINE_ID")
	if pipeline == "" {
		pipeline = repository
	}
	if execution == "" || stage == "" || pipeline == "" || repository == "" {
		return "", ErrInvalidPlanScope
	}

	parts := []string{
		firstEnv("HARNESS_ACCOUNT_ID"),
		firstEnv("HARNESS_ORG_ID"),
		firstEnv("HARNESS_PROJECT_ID"),
		pipeline,
		repository,
		execution,
		stage,
		firstEnv("HARNESS_STAGE_INDEX"),
		firstEnv("HARNESS_NODE_INDEX"),
	}
	return strings.Join(parts, "\x00"), nil
}

// autoPlanRoot prefers Harness' shared temp volume, which stays outside the
// repository checkout. Scratch retains compatibility with VM infrastructure,
// while the shared workspace is a last-resort fallback.
func autoPlanRoot() (string, error) {
	var root, source string
	for _, name := range []string{harnessTmpPathEnv, harnessScratchDirEnv, harnessWorkspaceEnv} {
		if root = firstEnv(name); root != "" {
			source = name
			break
		}
	}
	if root == "" {
		return "", fmt.Errorf("%w: none of %s, %s, or %s is set",
			ErrInvalidPlan, harnessTmpPathEnv, harnessScratchDirEnv, harnessWorkspaceEnv)
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", fmt.Errorf("%w: %s %q is not an absolute clean path", ErrInvalidPlan, source, root)
	}
	return root, nil
}

func autoPlanPath() (string, string, error) {
	scope, err := planScope()
	if err != nil {
		return "", "", err
	}
	root, err := autoPlanRoot()
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(scope))
	name := fmt.Sprintf("drone-cache-autodetect-%x.json", sum[:])
	return filepath.Join(root, autoPlanDirName, name), scope, nil
}

func securePlanDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: plan directory is not a regular directory", ErrInvalidPlan)
	}
	return os.Chmod(dir, 0o700)
}

func validatePlanFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: plan path is not a regular file", ErrInvalidPlan)
	}
	if info.Size() > autoPlanMaxSize {
		return fmt.Errorf("%w: plan exceeds %d bytes", ErrInvalidPlan, autoPlanMaxSize)
	}
	return nil
}

func validatePlan(plan AutoDetectPlan) error {
	if plan.Key == "" || len(plan.Key) > autoPlanMaxKeySize {
		return fmt.Errorf("%w: key is empty or too long", ErrInvalidPlan)
	}
	if len(plan.Sources) > autoPlanMaxSources {
		return fmt.Errorf("%w: too many source identities", ErrInvalidPlan)
	}
	for _, source := range plan.Sources {
		if len(source) != sha256.Size*2 {
			return fmt.Errorf("%w: malformed source identity", ErrInvalidPlan)
		}
		for _, char := range source {
			if !strings.ContainsRune("0123456789abcdef", char) {
				return fmt.Errorf("%w: malformed source identity", ErrInvalidPlan)
			}
		}
	}
	return nil
}

// WriteAutoDetectPlan atomically replaces the current execution's plan.
func WriteAutoDetectPlan(plan AutoDetectPlan) error {
	path, scope, err := autoPlanPath()
	if err != nil {
		return err
	}
	if err := validatePlan(plan); err != nil {
		return err
	}
	plan.Scope = scope
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if len(data) > autoPlanMaxSize {
		return fmt.Errorf("%w: encoded plan exceeds %d bytes", ErrInvalidPlan, autoPlanMaxSize)
	}
	if err := securePlanDir(path); err != nil {
		return err
	}

	if err := validatePlanFile(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".drone-cache-plan-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// ReadAutoDetectPlan returns the plan for the current execution.
func ReadAutoDetectPlan() (AutoDetectPlan, bool, error) {
	path, scope, err := autoPlanPath()
	if err != nil {
		return AutoDetectPlan{}, false, err
	}
	if err := validatePlanFile(path); err != nil {
		if os.IsNotExist(err) {
			return AutoDetectPlan{}, false, nil
		}
		return AutoDetectPlan{}, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return AutoDetectPlan{}, false, err
	}
	fileInfo, statErr := file.Stat()
	pathInfo, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil || !fileInfo.Mode().IsRegular() ||
		!pathInfo.Mode().IsRegular() || !os.SameFile(fileInfo, pathInfo) {
		file.Close()
		return AutoDetectPlan{}, false, fmt.Errorf("%w: plan changed while opening", ErrInvalidPlan)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, autoPlanMaxSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return AutoDetectPlan{}, false, readErr
	}
	if closeErr != nil {
		return AutoDetectPlan{}, false, closeErr
	}
	if len(data) > autoPlanMaxSize {
		return AutoDetectPlan{}, false, fmt.Errorf("%w: plan exceeds %d bytes", ErrInvalidPlan, autoPlanMaxSize)
	}

	var plan AutoDetectPlan
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return AutoDetectPlan{}, false, fmt.Errorf("%w: %v", ErrInvalidPlan, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AutoDetectPlan{}, false, fmt.Errorf("%w: trailing data", ErrInvalidPlan)
	}
	if err := validatePlan(plan); err != nil {
		return AutoDetectPlan{}, false, err
	}
	if plan.Scope != scope {
		_ = os.Remove(path)
		return AutoDetectPlan{}, false, ErrPlanScopeMismatch
	}
	return plan, true, nil
}

// RemoveAutoDetectPlan consumes the plan after a successful Save.
func RemoveAutoDetectPlan() error {
	path, _, err := autoPlanPath()
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: plan path is not a regular file", ErrInvalidPlan)
	}
	return os.Remove(path)
}
