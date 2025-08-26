//go:build linux

package tar

import (
    "os"
    "syscall"
    "time"
)

// extractAtime obtains the last access time from FileInfo on Linux.
func extractAtime(fi os.FileInfo) (time.Time, bool) {
    if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
        return time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec)), true
    }
    return time.Time{}, false
}

