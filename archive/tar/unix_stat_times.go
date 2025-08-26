//go:build !windows

package tar

import (
	"os"
	"syscall"
	"time"
)

func statTimes(fi os.FileInfo) (atime, ctime time.Time, ok bool) {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	return time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec), time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec), true
}
