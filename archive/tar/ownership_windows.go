//go:build windows

package tar

import "os"

// On Windows, POSIX ownership is not applicable.
func getOwnership(fi os.FileInfo) (int, int, bool) { return 0, 0, false }
func chown(path string, uid, gid int) error        { return nil }
func lchown(path string, uid, gid int) error       { return nil }

