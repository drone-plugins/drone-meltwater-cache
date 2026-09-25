package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/internal/plugin/autodetect"
	"github.com/meltwater/drone-cache/test"
)

// trackedNpmrc is the customer shape: a committed registry config with no cache key.
const trackedNpmrc = "" +
	"registry=https://example.harness.io/pkg/npm/npm/\n" +
	"//example.harness.io/pkg/npm/npm/:_authToken=${HAR_TOKEN}\n" +
	"ca=null\n"

func TestExecTrackedNpmrcStaysCleanAcrossRestoreAndSave(t *testing.T) {
	repo := newTrackedNpmRepo(t, trackedNpmrc)
	outside := t.TempDir()
	t.Setenv("HOME", outside)
	t.Setenv("HARNESS_WORKSPACE", repo)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "tracked-npmrc")

	dirs := detectNpmDirs(t)
	test.Equals(t, dirs, []string{filepath.Join(repo, "node_modules")})

	cacheRoot := t.TempDir()
	// Cold restore misses. Detection still runs first, which is where .npmrc used to be rewritten.
	_ = autoDetectPlugin(t, cacheRoot, true, false).Exec()
	assertGitClean(t, repo, trackedNpmrc)

	test.Ok(t, os.MkdirAll(filepath.Join(repo, "node_modules"), 0755))
	test.Ok(t, os.WriteFile(filepath.Join(repo, "node_modules", "index.js"), []byte("cached"), 0644))
	test.Ok(t, autoDetectPlugin(t, cacheRoot, false, true).Exec())
	assertGitClean(t, repo, trackedNpmrc)

	// Save runs detection again in the same checkout. A second pass must not append cache=.
	dirs = detectNpmDirs(t)
	test.Equals(t, dirs, []string{filepath.Join(repo, "node_modules")})
	assertGitClean(t, repo, trackedNpmrc)
}

func TestExecTrackedNpmrcStaysCleanWhenHomeIsCheckout(t *testing.T) {
	repo := newTrackedNpmRepo(t, trackedNpmrc)
	t.Setenv("HOME", repo)
	t.Setenv("HARNESS_WORKSPACE", repo)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "home-is-checkout")

	dirs := detectNpmDirs(t)
	test.Equals(t, dirs, []string{
		filepath.Join(repo, "node_modules"),
		filepath.Join(repo, ".npm"),
	})

	_ = autoDetectPlugin(t, t.TempDir(), true, false).Exec()
	assertGitClean(t, repo, trackedNpmrc)
	_, err := os.Stat(filepath.Join(repo, ".npm"))
	test.Assert(t, os.IsNotExist(err), "selecting ~/.npm must not create it in the checkout")
}

func TestExecTrackedNpmrcStaysCleanWhenCacheAlreadySet(t *testing.T) {
	original := trackedNpmrc + "cache=custom-npm-cache\n"
	repo := newTrackedNpmRepo(t, original)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HARNESS_WORKSPACE", repo)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "existing-cache")

	dirs := detectNpmDirs(t)
	test.Equals(t, dirs, []string{
		filepath.Join(repo, "node_modules"),
		filepath.Join(repo, "custom-npm-cache"),
	})

	_ = autoDetectPlugin(t, t.TempDir(), true, false).Exec()
	assertGitClean(t, repo, original)
}

func TestExecTrackedNpmrcStaysCleanWhenEnvSelectsCache(t *testing.T) {
	repo := newTrackedNpmRepo(t, trackedNpmrc)
	envCache := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HARNESS_WORKSPACE", repo)
	t.Setenv("npm_config_cache", envCache)
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "env-cache")

	dirs := detectNpmDirs(t)
	test.Equals(t, dirs, []string{filepath.Join(repo, "node_modules"), envCache})

	_ = autoDetectPlugin(t, t.TempDir(), true, false).Exec()
	assertGitClean(t, repo, trackedNpmrc)
}

func TestExecTrackedNpmrcWithoutTrailingNewlineStaysClean(t *testing.T) {
	original := strings.TrimRight(trackedNpmrc, "\n")
	repo := newTrackedNpmRepo(t, original)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HARNESS_WORKSPACE", repo)
	t.Setenv("npm_config_cache", "")
	t.Setenv("NPM_CONFIG_CACHE", "")
	t.Setenv("HARNESS_TMP_PATH", t.TempDir())
	setPluginPlanScope(t, "no-newline")

	_ = autoDetectPlugin(t, t.TempDir(), true, false).Exec()
	test.Ok(t, autoDetectPlugin(t, t.TempDir(), false, true).Exec())
	assertGitClean(t, repo, original)
}

func newTrackedNpmRepo(t *testing.T, npmrc string) string {
	t.Helper()

	repo := t.TempDir()
	test.Ok(t, os.WriteFile(filepath.Join(repo, ".npmrc"), []byte(npmrc), 0644))
	test.Ok(t, os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"name":"app","version":"1.0.0"}`), 0644))
	test.Ok(t, os.WriteFile(filepath.Join(repo, "package-lock.json"), []byte(`{"name":"app","lockfileVersion":3}`), 0644))
	// node_modules is ignored in real repos. .npmrc and .npm are not, so a plugin write still shows up.
	test.Ok(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n"), 0644))

	// CI images used for go test do not ship git. The byte and extra-path checks
	// below still cover the customer failure. When git is present, also commit
	// the fixture so git status --porcelain matches a real checkout.
	if gitBin, err := exec.LookPath("git"); err == nil {
		git := func(args ...string) {
			t.Helper()
			cmd := exec.Command(gitBin, args...)
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			test.Ok(t, err)
			if len(out) > 0 && !strings.Contains(string(out), "Initialized") {
				t.Logf("git %s: %s", strings.Join(args, " "), out)
			}
		}

		git("init", "-b", "master")
		git("config", "user.email", "cache-test@example.com")
		git("config", "user.name", "cache-test")
		git("add", ".npmrc", "package.json", "package-lock.json", ".gitignore")
		git("commit", "-m", "initial")
	}
	t.Chdir(repo)

	return repo
}

func detectNpmDirs(t *testing.T) []string {
	t.Helper()

	dirs, tools, _, err := autodetect.DetectDirectoriesToCache(false)
	test.Ok(t, err)
	test.Equals(t, tools, []string{"node"})

	return dirs
}

func assertGitClean(t *testing.T, repo, npmrc string) {
	t.Helper()

	assertFile(t, repo, ".npmrc", npmrc)
	assertFile(t, repo, "package.json", `{"name":"app","version":"1.0.0"}`)
	assertFile(t, repo, "package-lock.json", `{"name":"app","lockfileVersion":3}`)
	assertFile(t, repo, ".gitignore", "node_modules/\n")
	assertNoExtraCheckoutPaths(t, repo)

	gitBin, err := exec.LookPath("git")
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return
	}

	status := gitOutput(t, repo, gitBin, "status", "--porcelain")
	test.Assert(t, status == "", "expected a clean worktree, git status:\n%s", status)

	diff := gitOutput(t, repo, gitBin, "diff", "HEAD", "--", ".npmrc")
	test.Assert(t, diff == "", "expected no .npmrc diff, got:\n%s", diff)
}

func assertFile(t *testing.T, repo, name, want string) {
	t.Helper()

	got, err := os.ReadFile(filepath.Join(repo, name))
	test.Ok(t, err)
	test.Equals(t, string(got), want)
}

func assertNoExtraCheckoutPaths(t *testing.T, repo string) {
	t.Helper()

	allowed := map[string]struct{}{
		".npmrc":            {},
		"package.json":      {},
		"package-lock.json": {},
		".gitignore":        {},
	}

	err := filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == repo {
			return nil
		}

		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() && rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "node_modules" || strings.HasPrefix(rel, "node_modules"+string(filepath.Separator)) {
			if d.IsDir() && rel == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := allowed[rel]; !ok {
			t.Errorf("unexpected path in checkout: %s", rel)
		}

		return nil
	})
	test.Ok(t, err)
}

func gitOutput(t *testing.T, repo, gitBin string, args ...string) string {
	t.Helper()

	cmd := exec.Command(gitBin, args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	test.Ok(t, err)

	return string(out)
}
