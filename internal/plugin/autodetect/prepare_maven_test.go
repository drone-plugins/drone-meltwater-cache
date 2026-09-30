package autodetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMavenPreparerNoTrailingSpace(t *testing.T) {
	dir := t.TempDir()
	if _, err := newMavenPreparer().PrepareRepo(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".mvn", "maven.config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("line has trailing whitespace: %q", line)
		}
	}
}
