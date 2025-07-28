package tar

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	logger log.Logger

	root         string
	skipSymlinks bool
}

// New creates an archive that uses the .tar file format.
func New(logger log.Logger, root string, skipSymlinks bool) *Archive {
	return &Archive{logger, root, skipSymlinks}
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

		if err := filepath.Walk(src, writeToArchive(tw, a.root, a.skipSymlinks, &written, isRelativePath, a.logger)); err != nil {
			return written, fmt.Errorf("walk, add all files to archive, %w", err)
		}
	}

	return written, nil
}

// nolint: lll
func writeToArchive(tw *tar.Writer, root string, skipSymlinks bool, written *int64, isRelativePath bool, logger log.Logger) func(string, os.FileInfo, error) error {
	return func(path string, fi os.FileInfo, err error) error {
		level.Debug(logger).Log("path", path, "root", root) //nolint: errcheck

		if err != nil {
			return err
		}

		if fi == nil {
			return errors.New("no file info")
		}

		level.Info(logger).Log(
			"msg", "TAR Create: Preparing to archive",
			"local_path", path,
			"original_mod_time", fi.ModTime().Format(time.RFC3339Nano), // Original ModTime from local file
			"original_is_dir", fi.IsDir(),
			"original_size", fi.Size(),
			"original_mode", fi.Mode().String(),
		)

		// Create header for Regular files and Directories
		h, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return fmt.Errorf("create header for <%s>, %w", path, err)
		}

		if fi.Mode()&os.ModeSymlink != 0 { // isSymbolic
			if skipSymlinks {
				return nil
			}

			var err error
			if h, err = createSymlinkHeader(fi, path); err != nil {
				return fmt.Errorf("create header for symbolic link, %w", err)
			}
		}

		var name string
		if filepath.IsAbs(path) {
			name, err = filepath.Abs(path)
		} else if isRelativePath {
			name = path
		} else {
			name, err = relative(root, path)
		}

		if err != nil {
			return fmt.Errorf("relative name <%s>: <%s>, %w", path, root, err)
		}

		h.Name = name
		level.Info(logger).Log(
			"msg", "TAR Create: Writing header to archive",
			"archive_entry_name", h.Name,
			"header_mod_time", h.ModTime.Format(time.RFC3339Nano), // ModTime as stored in the header
			"header_type", h.Typeflag,
			"header_size", h.Size,
			"header_mode", os.FileMode(h.Mode).String(),
		)

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
		// Alternatives:
		// *written += h.FileInfo().Size()
		// *written += fi.Size()

		return nil
	}
}

func relative(parent string, path string) (string, error) {
	name := filepath.Base(path)

	rel, err := filepath.Rel(parent, filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("relative path <%s>, base <%s>, %w", rel, name, err)
	}

	// NOTICE: filepath.Rel puts "../" when given path is not under parent.
	for strings.HasPrefix(rel, "../") {
		rel = strings.TrimPrefix(rel, "../")
	}

	rel = filepath.ToSlash(rel)

	return strings.TrimPrefix(filepath.Join(rel, name), "/"), nil
}

func createSymlinkHeader(fi os.FileInfo, path string) (*tar.Header, error) {
	lnk, err := os.Readlink(path)
	if err != nil {
		return nil, fmt.Errorf("read link <%s>, %w", path, err)
	}

	h, err := tar.FileInfoHeader(fi, lnk)
	if err != nil {
		return nil, fmt.Errorf("create symlink header for <%s>, %w", path, err)
	}

	return h, nil
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
		written int64
		tr      = tar.NewReader(r)
	)

	for {
		h, err := tr.Next()

		switch {
		case err == io.EOF: // if no more files are found return
			return written, nil
		case err != nil: // return any other error
			return written, fmt.Errorf("tar reader <%v>, %w", err, ErrArchiveNotReadable)
		case h == nil: // if the header is nil, skip it
			continue
		}

		var target string
		if dst == h.Name || filepath.IsAbs(h.Name) {
			target = h.Name
		} else {
			name, err := relative(dst, h.Name)
			if err != nil {
				return 0, fmt.Errorf("relative name, %w", err)
			}

			target = filepath.Join(dst, name)
		}

		level.Debug(a.logger).Log("msg", "extracting archive", "path", target)

		level.Info(a.logger).Log(
			"msg", "TAR Extract: Processing archive entry",
			"archive_entry_name", h.Name,
			"target_local_path", target,
			"archive_mod_time_in_header", h.ModTime.Format(time.RFC3339Nano), // ModTime *from the tar header*
			"archive_type", h.Typeflag,
			"archive_size", h.Size,
			"archive_permissions", os.FileMode(h.Mode).String(),
		)

		if err := os.MkdirAll(filepath.Dir(target), defaultDirPermission); err != nil {
			return 0, fmt.Errorf("ensure directory <%s>, %w", target, err)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			level.Debug(a.logger).Log("msg", "TAR Extract: Handling explicit directory entry from archive", "path", target)
			if err := extractDir(h, target); err != nil {
				return written, err
			}
			level.Info(a.logger).Log("msg", "TAR Extract: Extracted explicit directory entry", "path", target)
			// --- POTENTIAL FIX: Set directory timestamps ---
			if err := os.Chtimes(target, h.ModTime, h.ModTime); err != nil {
				level.Error(a.logger).Log("msg", "TAR Extract: Failed to set directory times (explicit entry)", "path", target, "err", err)
			} else {
				level.Info(a.logger).Log("msg", "TAR Extract: Successfully set directory times (explicit entry)", "path", target, "set_mtime", h.ModTime.Format(time.RFC3339Nano))
			}
			continue
		case tar.TypeReg, tar.TypeRegA, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			n, err := extractRegular(h, tr, target)
			written += n

			if err != nil {
				return written, fmt.Errorf("extract regular file, %w", err)
			}
			level.Info(a.logger).Log("msg", "TAR Extract: Extracted regular file", "path", target, "bytes", n)
			// --- POTENTIAL FIX: Set regular file timestamps ---
			if err := os.Chtimes(target, h.ModTime, h.ModTime); err != nil {
				level.Error(a.logger).Log("msg", "TAR Extract: Failed to set file times", "path", target, "err", err)
			} else {
				level.Info(a.logger).Log("msg", "TAR Extract: Successfully set file times", "path", target, "set_mtime", h.ModTime.Format(time.RFC3339Nano))
			}
			level.Info(a.logger).Log("msg", "TAR Extract: Extracted symlink", "path", target, "link_target", h.Linkname)
			continue
		case tar.TypeSymlink:
			if err := extractSymlink(h, target); err != nil {
				return written, fmt.Errorf("extract symbolic link, %w", err)
			}
			level.Debug(a.logger).Log("msg", "TAR Extract: Extracted hard link", "path", target, "link_target", h.Linkname)

			continue
		case tar.TypeLink:
			if err := extractLink(h, target); err != nil {
				return written, fmt.Errorf("extract link, %w", err)
			}
			level.Info(a.logger).Log("msg", "TAR Extract: Extracted hard link", "path", target, "link_target", h.Linkname)

			continue
		case tar.TypeXGlobalHeader:
			level.Info(a.logger).Log("msg", "TAR Extract: Skipping global header")

			continue
		default:
			return written, fmt.Errorf("extract %s, unknown type flag: %c", target, h.Typeflag)
		}
	}
}

func extractDir(h *tar.Header, target string) error {
	if err := os.MkdirAll(target, os.FileMode(h.Mode)); err != nil {
		return fmt.Errorf("create directory <%s>, %w", target, err)
	}
	os.Chtimes(target, h.ModTime, h.ModTime)
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
	os.Chtimes(target, h.ModTime, h.ModTime)
	return written, nil
}

func extractSymlink(h *tar.Header, target string) error {
	if err := unlink(target); err != nil {
		return fmt.Errorf("unlink <%s>, %w", target, err)
	}

	if err := os.Symlink(h.Linkname, target); err != nil {
		return fmt.Errorf("create symbolic link <%s>, %w", target, err)
	}

	return nil
}

func extractLink(h *tar.Header, target string) error {
	if err := unlink(target); err != nil {
		return fmt.Errorf("unlink <%s>, %w", target, err)
	}

	if err := os.Link(h.Linkname, target); err != nil {
		return fmt.Errorf("create hard link <%s>, %w", h.Linkname, err)
	}

	return nil
}

func unlink(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return os.Remove(path)
	}

	return nil
}
