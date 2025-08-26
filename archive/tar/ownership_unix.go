//go:build !windows

package tar

import (
    "os"
    "syscall"
)

// getOwnership extracts uid/gid from FileInfo on Unix platforms.
func getOwnership(fi os.FileInfo) (int, int, bool) {
    if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
        return int(stat.Uid), int(stat.Gid), true
    }
    return 0, 0, false
}

// chown best-effort sets ownership; ignore errors at call sites.
func chown(path string, uid, gid int) error {
    // If uid/gid are zero and not desired, still attempt; caller ignores EPERM.
    return os.Chown(path, uid, gid)
}

// lchown best-effort sets ownership on a symlink.
func lchown(path string, uid, gid int) error {
    return os.Lchown(path, uid, gid)
}

