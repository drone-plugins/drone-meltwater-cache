//go:build windows

package tar

import (
    "os"
    "time"
)

// On Windows, access time retrieval varies; skip and rely on ModTime.
func extractAtime(fi os.FileInfo) (time.Time, bool) { return time.Time{}, false }

