package cache

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meltwater/drone-cache/archive"
	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/key"
	"github.com/meltwater/drone-cache/storage"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type rebuilder struct {
	logger log.Logger

	a  archive.Archive
	s  storage.Storage
	g  key.Generator
	fg key.Generator

	namespace         string
	override          bool
	missingPathPolicy MissingPathPolicy
}

type rebuildSummary struct {
	Requested     int
	Missing       int
	AlreadyExists int
	Scheduled     int
	Uploaded      int
	Failed        int
}

type uploadResult struct {
	Source string
	Target string
	Err    error
}

type scheduledUpload struct {
	src string
	dst string
}

// NewRebuilder creates a new cache.Rebuilder.
func NewRebuilder(logger log.Logger, s storage.Storage, a archive.Archive, g key.Generator, fg key.Generator, namespace string, override bool, missingPathPolicy MissingPathPolicy) Rebuilder { // nolint:lll
	return rebuilder{logger, a, s, g, fg, namespace, override, missingPathPolicy}
}

// normalizeDockerPath converts Docker infrastructure paths to a fixed format.
// When DRONE_STAGE_TYPE=DOCKER, paths like /tmp/harness/<uuid>/...
// are converted to docker/... for consistent remote storage keys.
func normalizeDockerPath(src string) string {
	if os.Getenv("DRONE_STAGE_TYPE") != "DOCKER" {
		return src
	}

	normalizedPath := filepath.ToSlash(src)
	const unixPrefix = "/tmp/harness/"
	const windowsPrefix = "C:/tmp/harness/"

	var remainder string
	if strings.HasPrefix(normalizedPath, unixPrefix) {
		remainder = strings.TrimPrefix(normalizedPath, unixPrefix)
	} else if strings.HasPrefix(normalizedPath, windowsPrefix) {
		remainder = strings.TrimPrefix(normalizedPath, windowsPrefix)
	} else {
		return src
	}

	// Find the UUID segment and skip it
	idx := strings.Index(remainder, "/")
	if idx == -1 {
		return src
	}

	// Return fixed prefix + relative path
	return filepath.Join("docker", remainder[idx+1:])
}

// Rebuild rebuilds cache from the files provided with given paths.
func (r rebuilder) Rebuild(srcs []string) error {
	level.Info(r.logger).Log("msg", "rebuilding cache")

	now := time.Now()
	summary := rebuildSummary{Requested: len(srcs)}

	if len(srcs) == 0 {
		if r.missingPathPolicy == MissingPathSkipRequirePresent {
			level.Error(r.logger).Log("msg", "cache save failed: no source paths configured", "requested", 0)
			return errors.New("cache save failed: no source paths configured")
		}
		level.Info(r.logger).Log("msg", "cache built", "took", time.Since(now))
		return nil
	}

	key, err := r.generateKey()
	if err != nil {
		return fmt.Errorf("generate key, %w", err)
	}

	namespace := filepath.ToSlash(filepath.Clean(r.namespace))
	scheduled := make([]scheduledUpload, 0, len(srcs))

	for _, src := range srcs {
		if _, err := os.Lstat(src); err != nil {
			if skip, fatalErr := r.handleMissingPath(src, err, &summary); fatalErr != nil {
				return fatalErr
			} else if skip {
				continue
			}
		}

		normalizedSrc := normalizeDockerPath(src)
		dst := filepath.Join(namespace, key, normalizedSrc)

		// If no override is set and object already exists in storage, skip it.
		if !r.override {
			exists, err := r.s.Exists(dst)
			if err != nil {
				return fmt.Errorf("destination <%s> existence check, %w", dst, err)
			}

			if exists {
				summary.AlreadyExists++
				continue
			}
		}

		level.Info(r.logger).Log("msg", "rebuilding cache for source path", "local", src)
		level.Debug(r.logger).Log("msg", "rebuilding cache for source path", "remote", dst)

		summary.Scheduled++
		scheduled = append(scheduled, scheduledUpload{src: src, dst: dst})
	}

	if summary.Scheduled == 0 {
		if r.missingPathPolicy == MissingPathSkipRequirePresent && summary.Missing == summary.Requested {
			level.Error(r.logger).Log("msg", "cache save failed: all configured source paths are missing",
				"requested", summary.Requested, "missing", summary.Missing)
			return errors.New("cache save failed: all configured source paths are missing")
		}

		r.logRebuildComplete(summary, now)
		return nil
	}

	results := make(chan uploadResult, len(scheduled))
	var wg sync.WaitGroup

	for _, item := range scheduled {
		wg.Add(1)
		go func(dst, src string) {
			defer wg.Done()
			results <- uploadResult{
				Source: src,
				Target: dst,
				Err:    r.rebuild(src, dst),
			}
		}(item.dst, item.src)
	}

	wg.Wait()
	close(results)

	errs := &internal.MultiError{}
	for result := range results {
		if result.Err != nil {
			summary.Failed++
			errs.Add(fmt.Errorf("upload from <%s> to <%s>, %w", result.Source, result.Target, result.Err))
			continue
		}
		summary.Uploaded++
	}

	if summary.Failed > 0 {
		level.Error(r.logger).Log("msg", "cache save failed",
			"requested", summary.Requested,
			"uploaded", summary.Uploaded,
			"existing", summary.AlreadyExists,
			"missing", summary.Missing,
			"failed", summary.Failed,
			"took", time.Since(now),
		)
		return fmt.Errorf("rebuild failed, %w", errs)
	}

	r.logRebuildComplete(summary, now)
	return nil
}

// handleMissingPath applies the missing-path policy for an Lstat failure.
// Returns (skip=true) when the path should be skipped, or a fatal error.
func (r rebuilder) handleMissingPath(src string, err error, summary *rebuildSummary) (bool, error) {
	switch r.missingPathPolicy {
	case MissingPathSkipRequirePresent:
		// Only genuine not-found errors may be skipped.
		if errors.Is(err, fs.ErrNotExist) {
			summary.Missing++
			level.Warn(r.logger).Log("msg", "cache source path does not exist; skipping", "path", src)
			return true, nil
		}
		return false, fmt.Errorf("source <%s>, make sure file or directory exists and readable, %w", src, err)

	case MissingPathSkipAllowEmpty:
		// Preserve automatic-detection behavior: skip any Lstat failure.
		summary.Missing++
		level.Warn(r.logger).Log("msg", fmt.Sprintf("source directory %s does not exist, skipping", src),
			"err", fmt.Errorf("source <%s>, make sure file or directory exists and readable, %w", src, err))
		return true, nil

	default: // MissingPathStrict
		return false, fmt.Errorf("source <%s>, make sure file or directory exists and readable, %w", src, err)
	}
}

func (r rebuilder) logRebuildComplete(summary rebuildSummary, started time.Time) {
	level.Info(r.logger).Log("msg", "cache save complete",
		"requested", summary.Requested,
		"uploaded", summary.Uploaded,
		"existing", summary.AlreadyExists,
		"missing", summary.Missing,
		"failed", summary.Failed,
		"took", time.Since(started),
	)
}

// rebuild pushes the archived file to the cache.
func (r rebuilder) rebuild(src, dst string) (err error) {
	isRelativePath := strings.HasPrefix(src, "./")
	level.Debug(r.logger).Log("msg", "rebuild", "src", src, "relativePath", isRelativePath) //nolint: errcheck
	src = filepath.Clean(src)
	if !isRelativePath {
		src, err = filepath.Abs(src)
		if err != nil {
			return fmt.Errorf("clean source path, %w", err)
		}
		level.Debug(r.logger).Log("msg", "src is adjusted", "src", src) //nolint: errcheck
	}

	pr, pw := io.Pipe()
	defer internal.CloseWithErrCapturef(&err, pr, "rebuild, pr close <%s>", src)

	var written int64

	go func(wrt *int64) {
		defer internal.CloseWithErrLogf(r.logger, pw, "pw close defer")

		level.Debug(r.logger).Log("msg", "caching paths", "src", src)

		localWritten, err := r.a.Create([]string{src}, pw, isRelativePath)
		if err != nil {
			if err := pw.CloseWithError(fmt.Errorf("archive write, pipe writer failed, %w", err)); err != nil {
				level.Error(r.logger).Log("msg", "pw close", "err", err)
			}
		}

		*wrt += localWritten
	}(&written)

	level.Debug(r.logger).Log("msg", "uploading archived directory", "local", src, "remote", dst)

	sw := &statWriter{}
	tr := io.TeeReader(pr, sw)

	if err := r.s.Put(dst, tr); err != nil {
		err = fmt.Errorf("upload file, pipe reader failed, %w", err)
		if err := pr.CloseWithError(err); err != nil {
			level.Error(r.logger).Log("msg", "pr close", "err", err)
		}

		return err
	}

	level.Info(r.logger).Log("msg", "uploaded cache", "src", src, "size before compression", humanize.Bytes(uint64(sw.written)), "size after compression", humanize.Bytes(uint64(written)))

	level.Debug(r.logger).Log(
		"msg", "archive created",
		"local", src,
		"remote", dst,
		"archived bytes", humanize.Bytes(uint64(sw.written)),
		"read bytes", humanize.Bytes(uint64(written)),
		"ratio", fmt.Sprintf("%%%0.2f", float64(sw.written)/float64(written)*100.0), // nolint:gomnd
	)

	return nil
}

// Helpers

func (r rebuilder) generateKey(parts ...string) (string, error) {
	key, err := r.g.Generate(parts...)
	if err == nil {
		return key, nil
	}

	if r.fg != nil {
		level.Error(r.logger).Log("msg", "falling back to fallback key generator", "err", err)

		key, err = r.fg.Generate(parts...)
		if err == nil {
			return key, nil
		}
	}

	return "", err
}
