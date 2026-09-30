package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meltwater/drone-cache/test"
)

func readGradleProperties(t *testing.T, dir string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(dir, "gradle.properties"))
	test.Ok(t, err)

	return string(content)
}

func assertInjected(t *testing.T, content, pathToCache string) {
	t.Helper()

	lines := strings.Split(content, "\n")
	test.Assert(t, contains(lines, "systemProp.gradle.user.home="+pathToCache),
		"systemProp.gradle.user.home should be on its own line, got %q", content)
	test.Assert(t, contains(lines, "org.gradle.caching=true"),
		"org.gradle.caching should be on its own line, got %q", content)
}

func contains(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}

	return false
}

func TestGradlePreparerFileWithoutTrailingNewline(t *testing.T) {
	dir, err := os.MkdirTemp("", "gradle-no-newline-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	// Customer file whose last line has no trailing newline.
	test.Ok(t, os.WriteFile(filepath.Join(dir, "gradle.properties"),
		[]byte("org.gradle.jvmargs=-Xmx2g\nrelease.stage=SNAPSHOT"), 0644))

	pathToCache, err := (&gradlePreparer{}).PrepareRepo(dir)
	test.Ok(t, err)
	test.Equals(t, filepath.Join(dir, ".gradle"), pathToCache)

	content := readGradleProperties(t, dir)

	test.Assert(t, !strings.Contains(content, "release.stage=SNAPSHOTsystemProp"),
		"injected property must not be concatenated onto the last customer property, got %q", content)
	test.Assert(t, contains(strings.Split(content, "\n"), "release.stage=SNAPSHOT"),
		"existing property must stay intact on its own line, got %q", content)
	assertInjected(t, content, pathToCache)
}

func TestGradlePreparerFileWithTrailingNewline(t *testing.T) {
	dir, err := os.MkdirTemp("", "gradle-newline-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	initial := "release.stage=SNAPSHOT\n"
	test.Ok(t, os.WriteFile(filepath.Join(dir, "gradle.properties"), []byte(initial), 0644))

	pathToCache, err := (&gradlePreparer{}).PrepareRepo(dir)
	test.Ok(t, err)

	content := readGradleProperties(t, dir)

	test.Assert(t, strings.HasPrefix(content, initial),
		"existing content must be preserved verbatim, got %q", content)
	test.Assert(t, !strings.Contains(content, "\n\n"),
		"no blank line should be introduced, got %q", content)
	assertInjected(t, content, pathToCache)
}

func TestGradlePreparerEmptyFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "gradle-empty-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	test.Ok(t, os.WriteFile(filepath.Join(dir, "gradle.properties"), []byte(""), 0644))

	pathToCache, err := (&gradlePreparer{}).PrepareRepo(dir)
	test.Ok(t, err)

	content := readGradleProperties(t, dir)

	test.Assert(t, !strings.HasPrefix(content, "\n"),
		"empty file should not gain a leading blank line, got %q", content)
	assertInjected(t, content, pathToCache)
}

func TestGradlePreparerCreatesMissingFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "gradle-create-*")
	test.Ok(t, err)
	defer os.RemoveAll(dir)

	pathToCache, err := (&gradlePreparer{}).PrepareRepo(dir)
	test.Ok(t, err)

	content := readGradleProperties(t, dir)

	test.Equals(t, 1, strings.Count(content, "org.gradle.caching=true"))
	assertInjected(t, content, pathToCache)
}
