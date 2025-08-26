//go:build darwin

package tar

import (
    "os"
    "syscall"
    "time"
)

// extractAtime obtains the last access time from FileInfo on Darwin.
func extractAtime(fi os.FileInfo) (time.Time, bool) {
    if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
        return time.Unix(int64(stat.Atimespec.Sec), int64(stat.Atimespec.Nsec)), true
    }
    return time.Time{}, false
}

