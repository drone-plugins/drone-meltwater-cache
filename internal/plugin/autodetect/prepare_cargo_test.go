package autodetect

import (
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestCargoPreparer_DefaultRepoLocalHome(t *testing.T) {
	t.Setenv("CARGO_HOME", "")
	tempDir := t.TempDir()

	cacheDir, err := newCargoPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)

	expected := filepath.Join(tempDir, ".cache", "cargo", "registry", "cache")
	test.Equals(t, expected, cacheDir)
}

func TestCargoPreparer_AdditionalCacheDirs_Default(t *testing.T) {
	t.Setenv("CARGO_HOME", "")
	tempDir := t.TempDir()

	dirs, err := cargoCacheDirs(tempDir)
	test.Ok(t, err)

	expected := []string{
		filepath.Join(tempDir, ".cache", "cargo", "registry", "index"),
		filepath.Join(tempDir, ".cache", "cargo", "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

func TestCargoPreparer_EnvOverride_Absolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CARGO_HOME", home)
	projectDir := t.TempDir()

	cacheDir, err := newCargoPreparer().PrepareRepo(projectDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(home, "registry", "cache"), cacheDir)

	dirs, err := cargoCacheDirs(projectDir)
	test.Ok(t, err)
	expected := []string{
		filepath.Join(home, "registry", "index"),
		filepath.Join(home, "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

func TestCargoPreparer_EnvOverride_Relative(t *testing.T) {
	t.Setenv("CARGO_HOME", "vendor/cargo")
	projectDir := t.TempDir()

	cacheDir, err := newCargoPreparer().PrepareRepo(projectDir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(projectDir, "vendor", "cargo", "registry", "cache"), cacheDir)
}

// Cargo relocation is env-only; the preparer must never write a config file,
// which would dirty the tracked tree and break clean-tree checks.
func TestCargoPreparer_WritesNoConfigFile(t *testing.T) {
	t.Setenv("CARGO_HOME", "")
	tempDir := t.TempDir()

	_, err := newCargoPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	_, err = cargoCacheDirs(tempDir)
	test.Ok(t, err)

	entries, err := filepathGlobAll(tempDir)
	test.Ok(t, err)
	test.Equals(t, 0, len(entries))
}

func TestCargoPreparer_Idempotent(t *testing.T) {
	t.Setenv("CARGO_HOME", "")
	tempDir := t.TempDir()

	first, err := newCargoPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	second, err := newCargoPreparer().PrepareRepo(tempDir)
	test.Ok(t, err)
	test.Equals(t, first, second)
}

// filepathGlobAll lists any entries created under dir, proving the preparer did
// not materialize config or cache files in the project tree.
func filepathGlobAll(dir string) ([]string, error) {
	return filepath.Glob(filepath.Join(dir, "*"))
}
