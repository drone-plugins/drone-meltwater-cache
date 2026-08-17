package autodetect

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func TestAutoKeySidecarRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	test.Ok(t, os.Mkdir(".git", 0755))

	_, ok, err := ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, !ok, "expected missing sidecar")

	test.Ok(t, WriteAutoKeySidecar(""))
	path, ok := autoKeyPath()
	test.Assert(t, ok, "expected sidecar path under .git")
	want, err := filepath.Abs(filepath.Join(".git", autoKeyFile))
	test.Ok(t, err)
	test.Equals(t, want, path)
	_, statErr := os.Stat(path)
	test.Assert(t, os.IsNotExist(statErr), "empty hashes must not write sidecar")

	test.Ok(t, WriteAutoKeySidecar("abc123"))
	got, ok, err := ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "expected sidecar")
	test.Equals(t, "abc123", got)

	info, err := os.Stat(filepath.Join(".git", autoKeyFile))
	test.Ok(t, err)
	test.Assert(t, !info.IsDir(), "sidecar must be a file under .git")
}

func TestAutoKeySidecarSkippedWithoutGitDir(t *testing.T) {
	t.Chdir(t.TempDir())

	test.Ok(t, WriteAutoKeySidecar("abc123"))
	_, ok, err := ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, !ok, "no .git means no sidecar")

	_, statErr := os.Stat(".cache-intelligence")
	test.Assert(t, os.IsNotExist(statErr), "must not write a workspace sidecar dir")
}

func TestAutoKeySidecarDoesNotDirtyGitStatus(t *testing.T) {
	requireGit(t)
	t.Chdir(t.TempDir())
	gitInit(t)

	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))
	gitAdd(t, "package.json")
	gitCommit(t, "init")

	test.Ok(t, WriteAutoKeySidecar("restore-hash"))
	status := gitStatusPorcelain(t)
	test.Equals(t, "", status)

	_, err := os.Stat(filepath.Join(".git", autoKeyFile))
	test.Ok(t, err)
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func gitInit(t *testing.T) {
	t.Helper()
	cmd := exec.Command("git", "init")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

func gitAdd(t *testing.T, paths ...string) {
	t.Helper()
	args := append([]string{"add"}, paths...)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
}

func gitCommit(t *testing.T, msg string) {
	t.Helper()
	cmd := exec.Command("git",
		"-c", "user.name=test",
		"-c", "user.email=test@test.local",
		"-c", "commit.gpgsign=false",
		"commit", "-m", msg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}

func gitStatusPorcelain(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return strings.TrimSpace(string(out))
}
