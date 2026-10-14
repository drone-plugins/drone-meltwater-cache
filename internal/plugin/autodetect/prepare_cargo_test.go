package autodetect

import (
	"path/filepath"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func isolateCargoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CARGO_HOME", "")
	t.Setenv("HARNESS_WORKSPACE", "")
}

func TestCargoPreparer_Default(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	preparer := newCargoPreparer()
	cacheDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, ".cache", "cargo", "registry", "cache")
	test.Equals(t, expected, cacheDir)
}

func TestCargoPreparer_EnvVarOverride_Relative(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	t.Setenv("CARGO_HOME", "custom_cargo")

	preparer := newCargoPreparer()
	cacheDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "custom_cargo", "registry", "cache")
	test.Equals(t, expected, cacheDir)
}

func TestCargoPreparer_EnvVarOverride_Absolute(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()
	absHome := filepath.Join(t.TempDir(), "abs_cargo")

	t.Setenv("CARGO_HOME", absHome)

	preparer := newCargoPreparer()
	cacheDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(absHome, "registry", "cache")
	test.Equals(t, expected, cacheDir)
}

func TestCargoCacheDirs_Default(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	extraDirs, err := cargoCacheDirs(dir)
	test.Ok(t, err)

	expected := []string{
		filepath.Join(dir, ".cache", "cargo", "registry", "index"),
		filepath.Join(dir, ".cache", "cargo", "git", "db"),
	}
	test.Equals(t, expected, extraDirs)
}

func TestCargoCacheDirs_EnvVarOverride_Absolute(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()
	absHome := filepath.Join(t.TempDir(), "abs_cargo")

	t.Setenv("CARGO_HOME", absHome)

	extraDirs, err := cargoCacheDirs(dir)
	test.Ok(t, err)

	expected := []string{
		filepath.Join(absHome, "registry", "index"),
		filepath.Join(absHome, "git", "db"),
	}
	test.Equals(t, expected, extraDirs)
}

func TestCargoPreparer_Idempotent_NoMutation(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	preparer := newCargoPreparer()
	first, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)
	second, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	test.Equals(t, first, second)

	// Preparer must not create any files in the project directory.
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	test.Ok(t, err)
	test.Assert(t, len(entries) == 0, "cargo preparer must not write to the project directory")
}
