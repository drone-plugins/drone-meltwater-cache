//go:build darwin

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
	atime := time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
	h.AccessTime = atime
	ctime := time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec)
	h.ChangeTime = ctime
}
