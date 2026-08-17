package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestExecRestoreWritesSidecarAndSaveReusesIt(t *testing.T) {
	npmCache := t.TempDir()
	t.Setenv("npm_config_cache", npmCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())
	test.Ok(t, os.Mkdir(".git", 0755))

	pkg := `{"name":"app","version":"1.0.0"}`
	test.Ok(t, os.WriteFile("package.json", []byte(pkg), 0644))
	cacheRoot := t.TempDir()

	p := autoDetectPlugin(t, cacheRoot, true, false, "")
	_ = p.Exec() // cold restore may miss; sidecar is written before fetch

	sidecar, ok, err := autodetect.ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "restore should write sidecar")
	_, err = os.Stat(filepath.Join(".git", "cache-intelligence-auto-key"))
	test.Ok(t, err)

	test.Ok(t, os.WriteFile("package-lock.json", []byte("generated-lock"), 0644))
	test.Ok(t, os.MkdirAll(filepath.Join("node_modules", "pkg"), 0755))
	test.Ok(t, os.WriteFile(filepath.Join("node_modules", "pkg", "index.js"), []byte("ok"), 0644))

	save := autoDetectPlugin(t, cacheRoot, false, true, "")
	test.Ok(t, save.Exec())

	after, ok, err := autodetect.ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "sidecar should remain for save")
	test.Equals(t, sidecar, after)

	var found int
	_ = filepath.Walk(cacheRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			found++
		}
		return nil
	})
	test.Assert(t, found > 0, "save should write cache objects")
}

func TestExecSaveWithoutSidecarIgnoresUntrackedLockfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	npmCache := t.TempDir()
	t.Setenv("npm_config_cache", npmCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())

	cmd := exec.Command("git", "init")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	pkg := `{"name":"app","version":"1.0.0"}`
	test.Ok(t, os.WriteFile("package.json", []byte(pkg), 0644))
	add := exec.Command("git", "add", "package.json")
	out, err = add.CombinedOutput()
	if err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	commit := exec.Command("git", "-c", "user.name=test", "-c", "user.email=test@test.local", "-c", "commit.gpgsign=false", "commit", "-m", "init")
	out, err = commit.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	cacheRoot := t.TempDir()
	restore := autoDetectPlugin(t, cacheRoot, true, false, "")
	_ = restore.Exec()

	sidecar, ok, err := autodetect.ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "restore should write sidecar")
	test.Ok(t, os.Remove(filepath.Join(".git", "cache-intelligence-auto-key")))

	test.Ok(t, os.WriteFile("package-lock.json", []byte("generated-lock"), 0644))
	test.Ok(t, os.MkdirAll(filepath.Join("node_modules", "pkg"), 0755))
	test.Ok(t, os.WriteFile(filepath.Join("node_modules", "pkg", "index.js"), []byte("ok"), 0644))

	save := autoDetectPlugin(t, cacheRoot, false, true, "")
	test.Ok(t, save.Exec())

	test.Ok(t, os.RemoveAll("node_modules"))
	restore2 := autoDetectPlugin(t, cacheRoot, true, false, "")
	test.Ok(t, restore2.Exec())

	data, err := os.ReadFile(filepath.Join("node_modules", "pkg", "index.js"))
	test.Ok(t, err)
	test.Equals(t, "ok", string(data))
	test.Equals(t, sidecar, mustReadSidecar(t))
}

func TestExecRestoreSidecarLeavesGitStatusClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	npmCache := t.TempDir()
	t.Setenv("npm_config_cache", npmCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())

	cmd := exec.Command("git", "init")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))
	add := exec.Command("git", "add", "package.json")
	out, err = add.CombinedOutput()
	if err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	commit := exec.Command("git", "-c", "user.name=test", "-c", "user.email=test@test.local", "-c", "commit.gpgsign=false", "commit", "-m", "init")
	out, err = commit.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	p := autoDetectPlugin(t, t.TempDir(), true, false, "")
	_ = p.Exec()

	_, err = os.Stat(filepath.Join(".git", "cache-intelligence-auto-key"))
	test.Ok(t, err)
	status := exec.Command("git", "status", "--porcelain")
	out, err = status.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	test.Equals(t, "", strings.TrimSpace(string(out)))
}

func TestExecCustomCacheKeyDoesNotUseSidecar(t *testing.T) {
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())
	test.Ok(t, os.Mkdir(".git", 0755))
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))
	test.Ok(t, autodetect.WriteAutoKeySidecar("sidecar-hash"))

	cacheRoot := t.TempDir()
	save := autoDetectPlugin(t, cacheRoot, false, true, "user-key")
	test.Ok(t, save.Exec())

	sidecar, ok, err := autodetect.ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "custom key must not consume/remove sidecar")
	test.Equals(t, "sidecar-hash", sidecar)
}

func TestExecSkipPrepareDoesNotWriteSidecar(t *testing.T) {
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Chdir(t.TempDir())
	test.Ok(t, os.Mkdir(".git", 0755))
	test.Ok(t, os.WriteFile("package.json", []byte(`{"name":"app"}`), 0644))
	empty := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(empty, "keep"), []byte("x"), 0644))

	cacheRoot := t.TempDir()
	p := autoDetectPlugin(t, cacheRoot, true, false, "")
	p.Config.Mount = []string{empty}
	_ = p.Exec()

	_, ok, err := autodetect.ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, !ok, "skipPrepare restore must not write sidecar")
}

func mustReadSidecar(t *testing.T) string {
	t.Helper()
	sidecar, ok, err := autodetect.ReadAutoKeySidecar()
	test.Ok(t, err)
	test.Assert(t, ok, "expected sidecar")
	return sidecar
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
