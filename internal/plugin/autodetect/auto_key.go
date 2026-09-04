package autodetect

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	harnessTmpPathEnv = "HARNESS_TMP_PATH"
	autoPlanDirName   = ".cache-intelligence"
	autoPlanMaxSize   = 64 * 1024
	autoPlanMaxKey    = 4096
	autoHashHexSize   = 32
)

var (
	ErrInvalidPlanScope  = errors.New("cache plan scope is incomplete")
	ErrInvalidPlan       = errors.New("invalid cache plan")
	ErrPlanScopeMismatch = errors.New("cache plan scope does not match this execution")
)

// AutoDetectPlan preserves the package.json decision made by Restore.
// PathIDs are hashes of autodetected paths, never paths trusted for archiving.
type AutoDetectPlan struct {
	Key     string   `json:"key"`
	Scope   string   `json:"scope"`
	PathIDs []string `json:"path_ids"`
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func planScope() (string, error) {
	execution := firstEnv("HARNESS_EXECUTION_ID", "HARNESS_BUILD_ID", "DRONE_BUILD_NUMBER")
	stage := firstEnv("HARNESS_STAGE_ID", "DRONE_STAGE_NUMBER", "DRONE_STAGE_NAME")
	if execution == "" || stage == "" {
		return "", ErrInvalidPlanScope
	}

	parts := []string{
		firstEnv("HARNESS_ACCOUNT_ID"),
		firstEnv("HARNESS_ORG_ID"),
		firstEnv("HARNESS_PROJECT_ID"),
		firstEnv("HARNESS_PIPELINE_ID"),
		execution,
		stage,
		firstEnv("HARNESS_STAGE_INDEX"),
		firstEnv("HARNESS_NODE_INDEX"),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))

	return fmt.Sprintf("%x", sum[:]), nil
}

func autoPlanPath() (string, string, error) {
	root := strings.TrimSpace(os.Getenv(harnessTmpPathEnv))
	if root == "" {
		return "", "", fmt.Errorf("%w: %s is not set", ErrInvalidPlan, harnessTmpPathEnv)
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("%w: %s must be absolute", ErrInvalidPlan, harnessTmpPathEnv)
	}

	scope, err := planScope()
	if err != nil {
		return "", "", err
	}

	return filepath.Join(root, autoPlanDirName, "drone-cache-autodetect-"+scope+".json"), scope, nil
}

func validatePlan(plan AutoDetectPlan) error {
	if len(plan.Key) == 0 || len(plan.Key) > autoPlanMaxKey || len(plan.Key)%autoHashHexSize != 0 {
		return fmt.Errorf("%w: malformed key", ErrInvalidPlan)
	}
	for _, char := range plan.Key {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return fmt.Errorf("%w: malformed key", ErrInvalidPlan)
		}
	}
	if len(plan.PathIDs) == 0 || len(plan.PathIDs) > 256 {
		return fmt.Errorf("%w: malformed path identities", ErrInvalidPlan)
	}
	for _, id := range plan.PathIDs {
		if len(id) != sha256.Size*2 {
			return fmt.Errorf("%w: malformed path identity", ErrInvalidPlan)
		}
		for _, char := range id {
			if !strings.ContainsRune("0123456789abcdef", char) {
				return fmt.Errorf("%w: malformed path identity", ErrInvalidPlan)
			}
		}
	}

	return nil
}

func validatePlanFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: plan is not a regular file", ErrInvalidPlan)
	}
	if info.Size() > autoPlanMaxSize {
		return fmt.Errorf("%w: plan is too large", ErrInvalidPlan)
	}

	return nil
}

// PathIdentities returns stable hashes for the paths selected by Restore.
func PathIdentities(paths []string) []string {
	ids := make([]string, 0, len(paths))
	for _, path := range paths {
		sum := sha256.Sum256([]byte(filepath.Clean(path)))
		ids = append(ids, fmt.Sprintf("%x", sum[:]))
	}
	sort.Strings(ids)

	return ids
}

// FilterPathsForPlan only removes paths; plan contents can never add a path.
func FilterPathsForPlan(paths, ids []string) []string {
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}

	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		sum := sha256.Sum256([]byte(filepath.Clean(path)))
		if _, ok := allowed[fmt.Sprintf("%x", sum[:])]; ok {
			filtered = append(filtered, path)
		}
	}

	return filtered
}

// WriteAutoDetectPlan atomically records the current execution's fallback.
func WriteAutoDetectPlan(plan AutoDetectPlan) error {
	path, scope, err := autoPlanPath()
	if err != nil {
		return err
	}
	plan.Scope = scope
	if err := validatePlan(plan); err != nil {
		return err
	}

	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if len(data) > autoPlanMaxSize {
		return fmt.Errorf("%w: encoded plan is too large", ErrInvalidPlan)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: plan directory is not a regular directory", ErrInvalidPlan)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}

	if err := validatePlanFile(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	file, err := os.CreateTemp(dir, ".drone-cache-plan-*")
	if err != nil {
		return err
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)

	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// ReadAutoDetectPlan returns the fallback recorded for this execution.
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
		return AutoDetectPlan{}, false, fmt.Errorf("%w: plan is too large", ErrInvalidPlan)
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
		return AutoDetectPlan{}, false, ErrPlanScopeMismatch
	}

	return plan, true, nil
}

// RemoveAutoDetectPlan consumes the current execution's fallback.
func RemoveAutoDetectPlan() error {
	path, _, err := autoPlanPath()
	if err != nil {
		return err
	}
	if err := validatePlanFile(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return os.Remove(path)
}
