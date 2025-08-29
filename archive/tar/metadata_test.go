package tar

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/test"
)

func TestExtractPreserveMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows metadata semantics differ")
	}
	src := t.TempDir()
	filePath := filepath.Join(src, "file.txt")
	data := []byte("hello")
	test.Ok(t, os.WriteFile(filePath, data, 0640))
	mtime := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	test.Ok(t, os.Chtimes(filePath, mtime, mtime))

	buf := new(bytes.Buffer)
	cwd, _ := os.Getwd()
	test.Ok(t, os.Chdir(src))
	_, err := New(log.NewNopLogger(), "", false, true).Create([]string{"file.txt"}, buf, true)
	test.Ok(t, os.Chdir(cwd))
	test.Ok(t, err)

	dst := t.TempDir()
	test.Ok(t, os.Chdir(dst))
	_, err = New(log.NewNopLogger(), "", false, true).Extract("", bytes.NewReader(buf.Bytes()))
	test.Ok(t, err)
	test.Ok(t, os.Chdir(cwd))

	extracted := filepath.Join(dst, "file.txt")
	fi, err := os.Stat(extracted)
	test.Ok(t, err)
	test.Equals(t, os.FileMode(0640), fi.Mode().Perm())
	test.Equals(t, mtime, fi.ModTime())
}

func TestExtractPreserveMetadata_OldArchiveGraceful(t *testing.T) {
	src := t.TempDir()
	filePath := filepath.Join(src, "file.txt")
	test.Ok(t, os.WriteFile(filePath, []byte("hi"), 0644))
	buf := new(bytes.Buffer)
	cwd, _ := os.Getwd()
	test.Ok(t, os.Chdir(src))
	_, err := New(log.NewNopLogger(), "", false, false).Create([]string{"file.txt"}, buf, true)
	test.Ok(t, os.Chdir(cwd))
	test.Ok(t, err)

	dst := t.TempDir()
	test.Ok(t, os.Chdir(dst))
	_, err = New(log.NewNopLogger(), "", false, true).Extract("", bytes.NewReader(buf.Bytes()))
	test.Ok(t, err)
	test.Ok(t, os.Chdir(cwd))
	test.Exists(t, filepath.Join(dst, "file.txt"))
}
