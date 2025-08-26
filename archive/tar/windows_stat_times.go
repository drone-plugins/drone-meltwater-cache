//go:build windows

package tar

import (
	"os"
	"time"
)

func statTimes(fi os.FileInfo) (atime, ctime time.Time, ok bool) {
	return time.Time{}, time.Time{}, false
}
