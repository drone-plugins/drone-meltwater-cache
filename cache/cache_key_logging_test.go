package cache

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/meltwater/drone-cache/key/generator"
)

func TestRebuildLogsCacheKey(t *testing.T) {
	const cacheKey = "expected-save-key"

	var buf bytes.Buffer
	logger := log.NewLogfmtLogger(&buf)

	src := createTempSource(t)
	storage := &MockStorage{
		PutFunc: func(p string, r io.Reader) error {
			_, _ = io.Copy(io.Discard, r)
			return nil
		},
	}

	r := NewRebuilder(logger, storage, &MockArchive{}, generator.NewStatic(cacheKey), nil, "ns", true, MissingPathSkipRequirePresent)
	if err := r.Rebuild([]string{src}); err != nil {
		t.Fatalf("Rebuild() error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"cache save using key",
		"key=" + cacheKey,
		"rebuilding cache for source path",
		"remote=ns/" + cacheKey + "/",
		"uploaded cache",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q\nfull log:\n%s", want, out)
		}
	}
}

func TestRestoreLogsCacheKey(t *testing.T) {
	const cacheKey = "expected-restore-key"

	var buf bytes.Buffer
	logger := log.NewLogfmtLogger(&buf)

	dstDir := filepath.Join(t.TempDir(), "vendor", "bundle")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	remote := filepath.ToSlash(filepath.Join("ns", cacheKey, dstDir))
	storage := &MockStorage{
		GetFunc: func(p string, w io.Writer) error {
			_, err := w.Write([]byte("test data"))
			return err
		},
	}

	r := NewRestorer(logger, storage, &MockArchive{}, generator.NewStatic(cacheKey), nil, "ns", false, false, true, "harness", "acct", "")
	if err := r.Restore([]string{dstDir}, ""); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"cache restore using key",
		"key=" + cacheKey,
		"restoring directory",
		"remote=" + remote,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q\nfull log:\n%s", want, out)
		}
	}
}
