package tar

import (
    "io/ioutil"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    "time"

    "github.com/go-kit/kit/log"
    "github.com/meltwater/drone-cache/test"
)

// Test that when preserve is enabled, file mode and times are restored (best-effort across OS).
func TestPreserveMetadata_FileModeAndTimes(t *testing.T) {
    test.Ok(t, os.MkdirAll(testRootMounted, 0755))
    test.Ok(t, os.MkdirAll(testRootExtracted, 0755))

    // Create a temp dir with one file whose mode/times we control.
    srcDir, cleanup := test.CreateTempDir(t, "preserve_file_meta_src", testRootMounted)
    t.Cleanup(cleanup)

    filePath := filepath.Join(srcDir, "file.txt")
    content := []byte("hello metadata\n")
    test.Ok(t, ioutil.WriteFile(filePath, content, 0640))

    // Set a known mtime (truncate to seconds for portability) and atime same as mtime
    wantTime := time.Now().Add(-7 * time.Minute).Truncate(time.Second)
    test.Ok(t, os.Chtimes(filePath, wantTime, wantTime))

    // Create tar with preservation enabled
    ta := NewWithPreserve(log.NewNopLogger(), testRootMounted, true, true)
    arcDir, arcCleanup := test.CreateTempDir(t, "preserve_file_meta_arc", testRootMounted)
    t.Cleanup(arcCleanup)
    archivePath := filepath.Join(arcDir, "preserve_file_meta.tar")

    written, err := create(ta, []string{srcDir}, archivePath)
    test.Ok(t, err)
    test.Assert(t, written > 0, "expected some bytes written")

    // Remove source to ensure we read from archive
    test.Ok(t, os.RemoveAll(srcDir))

    // Extract
    dstDir, dstCleanup := test.CreateTempDir(t, "preserve_file_meta_dst", testRootExtracted)
    t.Cleanup(dstCleanup)
    _, err = extract(ta, archivePath, dstDir)
    test.Ok(t, err)

    // Validate restored file metadata
    restored := filepath.Join(dstDir, filepath.Base(srcDir), "file.txt")
    st, err := os.Stat(restored)
    test.Ok(t, err)
    if runtime.GOOS != "windows" {
        // Mode check only meaningful on Unix-like systems
        // Mask to permission bits
        got := st.Mode().Perm()
        want := os.FileMode(0640)
        test.Equals(t, got, want)
    }
    // ModTime should be close to wantTime (allow small delta)
    if dt := st.ModTime().Sub(wantTime); dt < -2*time.Second || dt > 2*time.Second {
        t.Fatalf("modtime delta too large: got=%v want=%v", st.ModTime(), wantTime)
    }
}

// Test that directory times are restored after extraction completes (delayed restore).
func TestPreserveMetadata_DirectoryTimesDelayed(t *testing.T) {
    test.Ok(t, os.MkdirAll(testRootMounted, 0755))
    test.Ok(t, os.MkdirAll(testRootExtracted, 0755))

    // Create a nested structure
    srcDir, cleanup := test.CreateTempDir(t, "preserve_dir_meta_src", testRootMounted)
    t.Cleanup(cleanup)
    nested := filepath.Join(srcDir, "nested")
    test.Ok(t, os.MkdirAll(nested, 0755))
    nf := filepath.Join(nested, "f.txt")
    test.Ok(t, ioutil.WriteFile(nf, []byte("x"), 0644))

    // Set dir mtime
    wantDirTime := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
    test.Ok(t, os.Chtimes(nested, wantDirTime, wantDirTime))

    ta := NewWithPreserve(log.NewNopLogger(), testRootMounted, true, true)
    arcDir, arcCleanup := test.CreateTempDir(t, "preserve_dir_meta_arc", testRootMounted)
    t.Cleanup(arcCleanup)
    archivePath := filepath.Join(arcDir, "preserve_dir_meta.tar")
    _, err := create(ta, []string{srcDir}, archivePath)
    test.Ok(t, err)

    dstDir, dstCleanup := test.CreateTempDir(t, "preserve_dir_meta_dst", testRootExtracted)
    t.Cleanup(dstCleanup)
    _, err = extract(ta, archivePath, dstDir)
    test.Ok(t, err)

    restoredDir := filepath.Join(dstDir, filepath.Base(srcDir), "nested")
    st, err := os.Stat(restoredDir)
    test.Ok(t, err)
    if dt := st.ModTime().Sub(wantDirTime); dt < -2*time.Second || dt > 2*time.Second {
        t.Fatalf("dir modtime delta too large: got=%v want=%v", st.ModTime(), wantDirTime)
    }
}

// Test that enabling preserve on restore still works with archives created without preservation.
func TestPreserveMetadata_OldArchiveGraceful(t *testing.T) {
    test.Ok(t, os.MkdirAll(testRootMounted, 0755))
    test.Ok(t, os.MkdirAll(testRootExtracted, 0755))

    // Create a simple archive with preservation disabled
    srcs := exampleFileTree(t, "preserve_old_archive_src", testRootMounted)
    taNoPreserve := New(log.NewNopLogger(), testRootMounted, true)
    arcDir, arcCleanup := test.CreateTempDir(t, "preserve_old_archive_arc", testRootMounted)
    t.Cleanup(arcCleanup)
    archivePath := filepath.Join(arcDir, "old_archive.tar")
    _, err := create(taNoPreserve, srcs, archivePath)
    test.Ok(t, err)

    // Extract with preservation enabled; should not fail
    taPreserve := NewWithPreserve(log.NewNopLogger(), testRootMounted, true, true)
    dstDir, dstCleanup := test.CreateTempDir(t, "preserve_old_archive_dst", testRootExtracted)
    t.Cleanup(dstCleanup)
    _, err = extract(taPreserve, archivePath, dstDir)
    test.Ok(t, err)

    // Basic content equality still holds
    var relativeSrcs []string
    for _, s := range srcs {
        if !filepath.IsAbs(s) {
            relativeSrcs = append(relativeSrcs, s)
        }
    }
    test.EqualDirs(t, dstDir, testRootMounted, relativeSrcs)
}
