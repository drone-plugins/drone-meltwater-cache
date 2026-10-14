package autodetect

import (
	"os"
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

	t.Setenv("CARGO_HOME", "vendor/cargo")

	preparer := newCargoPreparer()
	cacheDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(dir, "vendor", "cargo", "registry", "cache")
	test.Equals(t, expected, cacheDir)
}

func TestCargoPreparer_EnvVarOverride_Absolute(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()
	absHome := filepath.Join(t.TempDir(), "cargo-home")

	t.Setenv("CARGO_HOME", absHome)

	preparer := newCargoPreparer()
	cacheDir, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)

	expected := filepath.Join(absHome, "registry", "cache")
	test.Equals(t, expected, cacheDir)
}

func TestCargoAdditionalCacheDirs_Default(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	dirs, err := cargoAdditionalCacheDirs(dir)
	test.Ok(t, err)

	expected := []string{
		filepath.Join(dir, ".cache", "cargo", "registry", "index"),
		filepath.Join(dir, ".cache", "cargo", "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

func TestCargoAdditionalCacheDirs_EnvVarOverride(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()
	absHome := filepath.Join(t.TempDir(), "cargo-home")

	t.Setenv("CARGO_HOME", absHome)

	dirs, err := cargoAdditionalCacheDirs(dir)
	test.Ok(t, err)

	expected := []string{
		filepath.Join(absHome, "registry", "index"),
		filepath.Join(absHome, "git", "db"),
	}
	test.Equals(t, expected, dirs)
}

// The preparer must never write any configuration file into the repository.
func TestCargoPreparer_ZeroMutation(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	manifest := filepath.Join(dir, "Cargo.toml")
	initial := []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\n")
	test.Ok(t, os.WriteFile(manifest, initial, 0644))

	preparer := newCargoPreparer()
	_, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)
	_, err = cargoAdditionalCacheDirs(dir)
	test.Ok(t, err)

	entries, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 1, len(entries))
	test.Equals(t, "Cargo.toml", entries[0].Name())

	after, err := os.ReadFile(manifest)
	test.Ok(t, err)
	test.Equals(t, string(initial), string(after))
}

// Running preparation twice must produce an identical result and no file writes.
func TestCargoPreparer_Idempotent(t *testing.T) {
	isolateCargoEnv(t)
	dir := t.TempDir()

	preparer := newCargoPreparer()
	first, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)
	second, err := preparer.PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, first, second)

	entries, err := os.ReadDir(dir)
	test.Ok(t, err)
	test.Equals(t, 0, len(entries))
}
