//go:build !windows

package tar

import (
	"archive/tar"
	"os"
	"syscall"
	"time"
)

func applyHeaderMetadata(fi os.FileInfo, h *tar.Header) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	h.Uid = int(st.Uid)
	h.Gid = int(st.Gid)
	atime := time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec))
	h.AccessTime = atime
	ctime := time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec))
	h.ChangeTime = ctime
}
