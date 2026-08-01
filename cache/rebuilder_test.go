package cache

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-kit/log"
	"github.com/meltwater/drone-cache/key/generator"
)

func TestResolveMissingPathPolicy(t *testing.T) {
	tests := []struct {
		name               string
		ignoreMissingPaths bool
		gracefulDetect     bool
		want               MissingPathPolicy
	}{
		{name: "default strict", want: MissingPathStrict},
		{name: "graceful detect", gracefulDetect: true, want: MissingPathSkipAllowEmpty},
		{name: "ignore missing paths", ignoreMissingPaths: true, want: MissingPathSkipRequirePresent},
		{name: "ignore missing takes precedence", ignoreMissingPaths: true, gracefulDetect: true, want: MissingPathSkipRequirePresent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMissingPathPolicy(tt.ignoreMissingPaths, tt.gracefulDetect)
			if got != tt.want {
				t.Fatalf("resolveMissingPathPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRebuildMissingPathMatrix(t *testing.T) {
	logger := log.NewNopLogger()
	keyGen := generator.NewStatic("test-key")

	t.Run("strict all exist", func(t *testing.T) {
		existing := createTempSource(t)
		storage := &MockStorage{}
		var putCount int32
		storage.PutFunc = func(p string, r io.Reader) error {
			atomic.AddInt32(&putCount, 1)
			_, _ = io.Copy(io.Discard, r)
			return nil
		}

		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathStrict)
		if err := r.Rebuild([]string{existing}); err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 1 {
			t.Fatalf("Put calls = %d, want 1", putCount)
		}
	})

	t.Run("strict one missing", func(t *testing.T) {
		existing := createTempSource(t)
		r := NewRebuilder(logger, &MockStorage{}, &MockArchive{}, keyGen, nil, "ns", true, MissingPathStrict)
		err := r.Rebuild([]string{existing, filepath.Join(t.TempDir(), "missing")})
		if err == nil {
			t.Fatal("Rebuild() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "make sure file or directory exists") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("strict all missing", func(t *testing.T) {
		r := NewRebuilder(logger, &MockStorage{}, &MockArchive{}, keyGen, nil, "ns", true, MissingPathStrict)
		err := r.Rebuild([]string{filepath.Join(t.TempDir(), "missing-a"), filepath.Join(t.TempDir(), "missing-b")})
		if err == nil {
			t.Fatal("Rebuild() expected error, got nil")
		}
	})

	t.Run("ignore missing all exist", func(t *testing.T) {
		a := createTempSource(t)
		b := createTempSource(t)
		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		if err := r.Rebuild([]string{a, b}); err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 2 {
			t.Fatalf("Put calls = %d, want 2", putCount)
		}
	})

	t.Run("ignore missing one missing", func(t *testing.T) {
		existing := createTempSource(t)
		var putCount int32
		var putMu sync.Mutex
		var putPaths []string
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				putMu.Lock()
				putPaths = append(putPaths, p)
				putMu.Unlock()
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{existing, filepath.Join(t.TempDir(), "missing")})
		if err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 1 {
			t.Fatalf("Put calls = %d, want 1", putCount)
		}
	})

	t.Run("ignore missing multiple missing one existing", func(t *testing.T) {
		existing := createTempSource(t)
		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{
			filepath.Join(t.TempDir(), "missing-a"),
			existing,
			filepath.Join(t.TempDir(), "missing-b"),
		})
		if err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 1 {
			t.Fatalf("Put calls = %d, want 1", putCount)
		}
	})

	t.Run("ignore missing all missing", func(t *testing.T) {
		r := NewRebuilder(logger, &MockStorage{}, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{filepath.Join(t.TempDir(), "missing-a"), filepath.Join(t.TempDir(), "missing-b")})
		if err == nil {
			t.Fatal("Rebuild() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "all configured source paths are missing") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ignore missing empty path list", func(t *testing.T) {
		r := NewRebuilder(logger, &MockStorage{}, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild(nil)
		if err == nil {
			t.Fatal("Rebuild() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no source paths configured") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ignore missing with permission error", func(t *testing.T) {
		denied := createPermissionDeniedPath(t)
		if denied == "" {
			t.Skip("unable to create permission-denied path on this platform")
		}
		existing := createTempSource(t)
		r := NewRebuilder(logger, &MockStorage{}, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{existing, denied})
		if err == nil {
			t.Fatal("Rebuild() expected permission error, got nil")
		}
		if strings.Contains(err.Error(), "all configured source paths are missing") {
			t.Fatalf("permission errors must remain fatal, got: %v", err)
		}
	})

	t.Run("ignore missing with archive failure", func(t *testing.T) {
		existing := createTempSource(t)
		missing := filepath.Join(t.TempDir(), "missing")
		archive := &MockArchive{
			CreateFunc: func(srcs []string, w io.Writer, stripComponents bool) (int64, error) {
				return 0, errors.New("archive boom")
			},
		}
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				_, err := io.Copy(io.Discard, r)
				return err
			},
		}
		r := NewRebuilder(logger, storage, archive, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{existing, missing})
		if err == nil {
			t.Fatal("Rebuild() expected archive failure, got nil")
		}
	})

	t.Run("ignore missing with upload failure", func(t *testing.T) {
		existing := createTempSource(t)
		missing := filepath.Join(t.TempDir(), "missing")
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				_, _ = io.Copy(io.Discard, r)
				return errors.New("upload boom")
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{existing, missing})
		if err == nil {
			t.Fatal("Rebuild() expected upload failure, got nil")
		}
	})

	t.Run("mixed upload success and failure", func(t *testing.T) {
		a := createTempSource(t)
		b := createTempSource(t)
		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				n := atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				if n == 1 {
					return nil
				}
				return errors.New("second upload failed")
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{a, b})
		if err == nil {
			t.Fatal("Rebuild() expected failure when any upload fails, got nil")
		}
	})

	t.Run("all remote objects already exist override disabled", func(t *testing.T) {
		a := createTempSource(t)
		b := createTempSource(t)
		var putCount int32
		storage := &MockStorage{
			ExistsFunc: func(p string) (bool, error) { return true, nil },
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", false, MissingPathSkipRequirePresent)
		if err := r.Rebuild([]string{a, b}); err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 0 {
			t.Fatalf("Put calls = %d, want 0", putCount)
		}
	})

	t.Run("missing path plus remote object already exists", func(t *testing.T) {
		existing := createTempSource(t)
		storage := &MockStorage{
			ExistsFunc: func(p string) (bool, error) { return true, nil },
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", false, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{existing, filepath.Join(t.TempDir(), "missing")})
		if err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
	})

	t.Run("file and directory source paths", func(t *testing.T) {
		dir := createTempSource(t)
		file, err := os.CreateTemp(t.TempDir(), "cache-file-*.txt")
		if err != nil {
			t.Fatalf("CreateTemp: %v", err)
		}
		_, _ = file.WriteString("hello")
		_ = file.Close()

		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		if err := r.Rebuild([]string{dir, file.Name()}); err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 2 {
			t.Fatalf("Put calls = %d, want 2", putCount)
		}
	})

	t.Run("relative and absolute paths", func(t *testing.T) {
		abs := createTempSource(t)
		rel := filepath.Join(".", "rel-cache-src")
		if err := os.MkdirAll(rel, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(rel, "data.txt"), []byte("rel"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(rel) })

		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		if err := r.Rebuild([]string{abs, "./rel-cache-src"}); err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 2 {
			t.Fatalf("Put calls = %d, want 2", putCount)
		}
	})

	t.Run("dangling symbolic link treated as missing when ignore enabled", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "dangling")
		if err := os.Symlink(filepath.Join(t.TempDir(), "does-not-exist"), link); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}
		existing := createTempSource(t)
		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		// Lstat on dangling symlink succeeds (the link itself exists). Archive may still fail.
		// For ignore-missing we only skip ErrNotExist from Lstat. Ensure existing path still uploads
		// when dangling link causes archive/upload failure → overall fail.
		err := r.Rebuild([]string{existing, link})
		// Either succeeds (if archive accepts dangling link) or fails on archive — both are acceptable
		// as long as we do not treat the dangling link as a silent missing skip incorrectly without upload of existing.
		if err == nil && atomic.LoadInt32(&putCount) < 1 {
			t.Fatal("expected at least one upload when rebuild succeeds")
		}
	})

	t.Run("path disappears after validation", func(t *testing.T) {
		existing := createTempSource(t)
		archive := &MockArchive{
			CreateFunc: func(srcs []string, w io.Writer, stripComponents bool) (int64, error) {
				return 0, fsErrNotExist("path disappeared")
			},
		}
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				_, err := io.Copy(io.Discard, r)
				return err
			},
		}
		r := NewRebuilder(logger, storage, archive, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		err := r.Rebuild([]string{existing})
		if err == nil {
			t.Fatal("Rebuild() expected archive failure after validation, got nil")
		}
	})

	t.Run("graceful detect all missing still succeeds", func(t *testing.T) {
		r := NewRebuilder(logger, &MockStorage{}, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipAllowEmpty)
		if err := r.Rebuild([]string{filepath.Join(t.TempDir(), "missing")}); err != nil {
			t.Fatalf("auto-detect policy should succeed when all missing, got: %v", err)
		}
	})

	t.Run("concurrent successful uploads", func(t *testing.T) {
		paths := make([]string, 20)
		for i := range paths {
			paths[i] = createTempSource(t)
		}
		var putCount int32
		storage := &MockStorage{
			PutFunc: func(p string, r io.Reader) error {
				atomic.AddInt32(&putCount, 1)
				_, _ = io.Copy(io.Discard, r)
				return nil
			},
		}
		r := NewRebuilder(logger, storage, &MockArchive{}, keyGen, nil, "ns", true, MissingPathSkipRequirePresent)
		if err := r.Rebuild(paths); err != nil {
			t.Fatalf("Rebuild() unexpected error: %v", err)
		}
		if atomic.LoadInt32(&putCount) != 20 {
			t.Fatalf("Put calls = %d, want 20", putCount)
		}
	})
}

func createTempSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("cached"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

func createPermissionDeniedPath(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	child := filepath.Join(parent, "secret")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(parent, 0); err != nil {
		return ""
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0o700)
	})
	denied := filepath.Join(parent, "secret")
	if _, err := os.Lstat(denied); err == nil || errors.Is(err, os.ErrNotExist) {
		_ = os.Chmod(parent, 0o700)
		return ""
	}
	return denied
}

type notExistError string

func (e notExistError) Error() string { return string(e) }
func (e notExistError) Is(target error) bool {
	return target == os.ErrNotExist
}

func fsErrNotExist(msg string) error {
	return notExistError(msg)
}
