package tar

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/meltwater/drone-cache/internal"
)

const defaultDirPermission = 0755

var (
	// ErrSourceNotReachable means that given source is not reachable.
	ErrSourceNotReachable = errors.New("source not reachable")
	// ErrArchiveNotReadable means that given archive not readable/corrupted.
	ErrArchiveNotReadable = errors.New("archive not readable")
)

// Archive implements archive for tar.
type Archive struct {
	logger           log.Logger
	root             string
	skipSymlinks     bool
	preserveMetadata bool
}

// New creates an archive that uses the .tar file format.
func New(logger log.Logger, root string, skipSymlinks, preserveMetadata bool) *Archive {
	return &Archive{logger, root, skipSymlinks, preserveMetadata}
}

// Create writes content of the given source to an archive, returns written bytes.
// If isRelativePath is true, it clones using the path, else it clones using a path
// combining archive's root with the path.
func (a *Archive) Create(srcs []string, w io.Writer, isRelativePath bool) (int64, error) {
	tw := tar.NewWriter(w)
	defer internal.CloseWithErrLogf(a.logger, tw, "tar writer")

	var written int64

	for _, src := range srcs {
		_, err := os.Lstat(src)
		if err != nil {
			return written, fmt.Errorf("make sure file or directory readable <%s>: %v,, %w", src, err, ErrSourceNotReachable)
		}

		if err := filepath.Walk(src, a.writeToArchive(tw, &written, isRelativePath)); err != nil {
			return written, fmt.Errorf("walk, add all files to archive, %w", err)
		}
	}

	return written, nil
}

func (a *Archive) writeToArchive(tw *tar.Writer, written *int64, isRelativePath bool) func(string, os.FileInfo, error) error {
	return func(path string, fi os.FileInfo, err error) error {
		level.Debug(a.logger).Log("path", path, "root", a.root) //nolint: errcheck

		if err != nil {
			return err
		}

		if fi == nil {
			return errors.New("no file info")
		}

		var linkName string
		if fi.Mode()&os.ModeSymlink != 0 {
			if a.skipSymlinks {
				return nil
			}
			linkName, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read link <%s>, %w", path, err)
			}
		}

		h, err := tar.FileInfoHeader(fi, linkName)
		if err != nil {
			return fmt.Errorf("create header for <%s>, %w", path, err)
		}

		if a.preserveMetadata {
			h.Format = tar.FormatPAX
			if atime, ctime, ok := statTimes(fi); ok {
				h.AccessTime = atime
				h.ChangeTime = ctime
			}
		}

		var name string
		if filepath.IsAbs(path) {
			name, err = filepath.Abs(path)
		} else if isRelativePath {
			name = path
		} else {
			name, err = relative(a.root, path)
		}

		if err != nil {
			return fmt.Errorf("relative name <%s>: <%s>, %w", path, a.root, err)
		}

		h.Name = name

		if err := tw.WriteHeader(h); err != nil {
			return fmt.Errorf("write header for <%s>, %w", path, err)
		}

		if !fi.Mode().IsRegular() {
			return nil
		}

		n, err := writeFileToArchive(tw, path)
		if err != nil {
			return fmt.Errorf("write file to archive, %w", err)
		}

		*written += n
		return nil
	}
}

func writeFileToArchive(tw io.Writer, path string) (n int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open file <%s>, %w", path, err)
	}

	defer internal.CloseWithErrCapturef(&err, f, "write file to archive <%s>", path)

	written, err := io.Copy(tw, f)
	if err != nil {
		return written, fmt.Errorf("copy the file <%s> data to the tarball, %w", path, err)
	}

	return written, nil
}

// Extract reads content from the given archive reader and restores it to the destination, returns written bytes.
func (a *Archive) Extract(dst string, r io.Reader) (int64, error) {
	var (
		written    int64
		tr         = tar.NewReader(r)
		dirs       []*tar.Header
	)

	for {
		h, err := tr.Next()

		switch {
		case err == io.EOF:
			return written, a.restoreDirs(dirs)
		case err != nil:
			return written, fmt.Errorf("tar reader <%v>, %w", err, ErrArchiveNotReadable)
		case h == nil:
			continue
		}

		target, err := a.calcTargetPath(dst, h.Name)
		if err != nil {
			return written, err
		}

		level.Debug(a.logger).Log("msg", "extracting archive", "path", target)

		if err := os.MkdirAll(filepath.Dir(target), defaultDirPermission); err != nil {
			return 0, fmt.Errorf("ensure directory <%s>, %w", target, err)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := extractDir(h, target); err != nil {
				return written, err
			}
			if a.preserveMetadata {
				dirs = append(dirs, h)
			}
		case tar.TypeReg, tar.TypeRegA, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			n, err := extractRegular(h, tr, target)
			written += n
			if err != nil {
				return written, fmt.Errorf("extract regular file, %w", err)
			}
			if a.preserveMetadata {
				if err := a.restoreFileMetadata(h, target); err != nil {
					level.Warn(a.logger).Log("msg", "could not restore metadata", "err", err)
				}
			}
		case tar.TypeSymlink:
			if err := extractSymlink(h, target); err != nil {
				return written, fmt.Errorf("extract symbolic link, %w", err)
			}
			if a.preserveMetadata {
				if err := a.restoreSymlinkMetadata(h, target); err != nil {
					level.Warn(a.logger).Log("msg", "could not restore symlink metadata", "err", err)
				}
			}
		case tar.TypeLink:
			if err := extractLink(h, target); err != nil {
				return written, fmt.Errorf("extract link, %w", err)
			}
		case tar.TypeXGlobalHeader:
			continue
		default:
			return written, fmt.Errorf("extract %s, unknown type flag: %c", target, h.Typeflag)
		}
	}
}

func (a *Archive) calcTargetPath(dst, hName string) (string, error) {
	var target string
	if dst == hName || filepath.IsAbs(hName) {
		target = hName
	} else {
		name, err := relative(dst, hName)
		if err != nil {
			return "", fmt.Errorf("relative name, %w", err)
		}
		target = filepath.Join(dst, name)
	}
	return target, nil
}

func (a *Archive) restoreDirs(dirs []*tar.Header) error {
	for i := len(dirs) - 1; i >= 0; i-- {
		h := dirs[i]
		target, err := a.calcTargetPath("", h.Name) // Dirs are stored with full path
		if err != nil {
			return err
		}
		if err := a.restoreDirMetadata(h, target); err != nil {
			level.Warn(a.logger).Log("msg", "could not restore dir metadata", "err", err)
		}
	}
	return nil
}

func (a *Archive) restoreFileMetadata(h *tar.Header, target string) error {
	if err := os.Chmod(target, os.FileMode(h.Mode)); err != nil {
		return err
	}
	atime := h.AccessTime
	if atime.IsZero() {
		atime = h.ModTime
	}
	if err := os.Chtimes(target, atime, h.ModTime); err != nil {
		return err
	}
	return chown(target, h.Uid, h.Gid)
}

func (a *Archive) restoreDirMetadata(h *tar.Header, target string) error {
	if err := os.Chmod(target, os.FileMode(h.Mode)); err != nil {
		return err
	}
	atime := h.AccessTime
	if atime.IsZero() {
		atime = h.ModTime
	}
	if err := os.Chtimes(target, atime, h.ModTime); err != nil {
		return err
	}
	return chown(target, h.Uid, h.Gid)
}

func (a *Archive) restoreSymlinkMetadata(h *tar.Header, target string) error {
	return lchown(target, h.Uid, h.Gid)
}

func extractDir(h *tar.Header, target string) error {
	if err := os.MkdirAll(target, os.FileMode(h.Mode)); err != nil {
		return fmt.Errorf("create directory <%s>, %w", target, err)
	}
	return nil
}

func extractRegular(h *tar.Header, tr io.Reader, target string) (n int64, err error) {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(h.Mode))
	if err != nil {
		return 0, fmt.Errorf("open extracted file for writing <%s>, %w", target, err)
	}
	defer internal.CloseWithErrCapturef(&err, f, "extract regular <%s>", target)

	written, err := io.Copy(f, tr)
	if err != nil {
		return written, fmt.Errorf("copy extracted file for writing <%s>, %w", target, err)
	}
	return written, nil
}

func extractSymlink(h *tar.Header, target string) error {
	if err := unlink(target); err != nil {
		return fmt.Errorf("unlink <%s>, %w", target, err)
	}
	return os.Symlink(h.Linkname, target)
}

func extractLink(h *tar.Header, target string) error {
	if err := unlink(target); err != nil {
		return fmt.Errorf("unlink <%s>, %w", target, err)
	}
	return os.Link(h.Linkname, target)
}

func unlink(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return os.Remove(path)
	}
	return nil
}

func relative(parent string, path string) (string, error) {
	name := filepath.Base(path)

	rel, err := filepath.Rel(parent, filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("relative path <%s>, base <%s>, %w", rel, name, err)
	}

	for strings.HasPrefix(rel, "../") {
		rel = strings.TrimPrefix(rel, "../")
	}

	rel = filepath.ToSlash(rel)

	return strings.TrimPrefix(filepath.Join(rel, name), "/"), nil
}
