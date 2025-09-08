//go:build windows

package tar

import (
	"archive/tar"
	"os"
)

func applyHeaderMetadata(fi os.FileInfo, h *tar.Header) {}
