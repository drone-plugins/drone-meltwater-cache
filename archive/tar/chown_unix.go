//go:build !windows

package tar

import "os"

func chown(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

func lchown(path string, uid, gid int) error {
	return os.Lchown(path, uid, gid)
}
