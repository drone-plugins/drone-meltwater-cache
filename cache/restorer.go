package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/meltwater/drone-cache/archive"
	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/key"
	"github.com/meltwater/drone-cache/storage"
	"github.com/meltwater/drone-cache/storage/common"
)

type restorer struct {
	logger log.Logger

	a  archive.Archive
	s  storage.Storage
	g  key.Generator
	fg key.Generator

	namespace               string
	failIfKeyNotPresent     bool
	enableCacheKeySeparator bool
	backend                 string
	accountID               string
}

var cacheFileMutex sync.Mutex // To ensure thread-safe writes to the file

// NewRestorer creates a new cache.Restorer.
func NewRestorer(logger log.Logger, s storage.Storage, a archive.Archive, g key.Generator, fg key.Generator, namespace string, failIfKeyNotPresent bool, enableCacheKeySeparator bool, backend, accountID string) Restorer { // nolint:lll
	return restorer{logger, a, s, g, fg, namespace, failIfKeyNotPresent, enableCacheKeySeparator, backend, accountID}
}

// Restore restores files from the cache provided with given paths.
func (r restorer) Restore(dsts []string, cacheFileName string) error {
	level.Info(r.logger).Log("msg", "restoring cache")

	now := time.Now()

	key, err := r.generateKey()
	if err != nil {
		return fmt.Errorf("generate key, %w", err)
	}

	var (
		wg        sync.WaitGroup
		errs      = &internal.MultiError{}
		namespace = filepath.ToSlash(filepath.Clean(r.namespace))
	)

	// A map to store the original paths for each destination
	sourcePaths := make(map[string]string)

	if len(dsts) == 0 {
		prefix := filepath.Join(namespace, key)
		if !strings.HasSuffix(prefix, getSeparator()) && r.enableCacheKeySeparator {
			prefix = prefix + getSeparator()
		}
		entries, err := r.s.List(prefix)

		if err == nil {
			if r.failIfKeyNotPresent && len(entries) == 0 {
				return fmt.Errorf("key %s does not exist", prefix)
			}
			if r.backend == "harness" {
				prefix = r.accountID + "/intel/" + prefix
			}

			for _, e := range entries {
				entryPath := e.Path
				origPath := entryPath // Save original for source path mapping
				var dst string

				level.Info(r.logger).Log("msg", "processing entry", "entryPath", entryPath, "prefix", prefix, "key", key)
				
				// First, attempt standard trimming based on separator settings
				if r.enableCacheKeySeparator {
					dst = strings.TrimPrefix(entryPath, prefix)
				} else {
					dst = strings.TrimPrefix(entryPath, prefix+getSeparator())
				}
				
				level.Info(r.logger).Log("msg", "after initial trim", "dst", dst, "entryPath", entryPath)

				// If path wasn't correctly trimmed (prefix didn't match exactly)
				if strings.HasPrefix(dst, namespace) || strings.Contains(dst, key) {
					level.Info(r.logger).Log("msg", "path needs special processing", "dst", dst)
					
					// Extract just the path portion by identifying where keys are used
					pathComponents := strings.Split(entryPath, getSeparator())
					level.Info(r.logger).Log("msg", "path components", "components", fmt.Sprintf("%v", pathComponents))
					
					// Find where the key part ends and real path starts
					keyComponentIndex := -1
					for i, component := range pathComponents {
						if strings.Contains(component, key) {
							keyComponentIndex = i
							level.Info(r.logger).Log("msg", "found key component", "index", i, "component", component)
						}
					}
					
					// If we found the key component, extract just the path after it
					if keyComponentIndex >= 0 && keyComponentIndex+1 < len(pathComponents) {
						// Extract ONLY the path portion - this is the critical change
						pathOnly := strings.Join(pathComponents[keyComponentIndex+1:], getSeparator())
						level.Info(r.logger).Log("msg", "extracted pure path", "pathOnly", pathOnly)
						dst = pathOnly
					} else {
						level.Info(r.logger).Log("msg", "couldn't extract path", "keyComponentIndex", keyComponentIndex, "totalComponents", len(pathComponents))
					}
				}

				if dst != "" {
					level.Info(r.logger).Log("msg", "adding to destinations", "dst", dst, "sourcePath", origPath)
					dsts = append(dsts, dst)
					sourcePaths[dst] = origPath
				} else {
					level.Info(r.logger).Log("msg", "skipping empty destination", "entryPath", entryPath)
				}
			}
		} else if err != common.ErrNotImplemented {
			return err
		}
	}

	for _, dst := range dsts {
		var src string
		if originalPath, exists := sourcePaths[dst]; exists {
			src = originalPath
		} else {
			src = filepath.Join(namespace, key, dst)
		}

		level.Info(r.logger).Log("msg", "restoring directory", "local", dst, "remote", src)
		level.Debug(r.logger).Log("msg", "restoring directory", "remote", src)

		wg.Add(1)

		go func(src, dst string) {
			defer wg.Done()

			if err := r.restore(src, dst, cacheFileName); err != nil {
				errs.Add(fmt.Errorf("download from <%s> to <%s>, %w", src, dst, err))
			}
		}(src, dst)
	}

	wg.Wait()

	if errs.Err() != nil {
		return fmt.Errorf("restore failed, %w", errs)
	}

	level.Info(r.logger).Log("msg", "cache restored", "took", time.Since(now))

	return nil
}

// restore fetches the archived file from the cache and restores to the host machine's file system.
func (r restorer) restore(src, dst, cacheFileName string) (err error) {
	pr, pw := io.Pipe()
	defer internal.CloseWithErrCapturef(&err, pr, "rebuild, pr close <%s>", dst)

	go func() {
		defer internal.CloseWithErrLogf(r.logger, pw, "pw close defer")

		level.Debug(r.logger).Log("msg", "downloading archived directory", "remote", src, "local", dst)

		if err := r.s.Get(src, pw); err != nil {
			if err := pw.CloseWithError(fmt.Errorf("get file from storage backend, pipe writer failed, %w", err)); err != nil {
				level.Error(r.logger).Log("msg", "pw close", "err", err)
			}
		}
	}()

	level.Debug(r.logger).Log("msg", "extracting archived directory", "remote", src, "local", dst)

	written, err := r.a.Extract(dst, pr)
	if err != nil {
		err = fmt.Errorf("extract files from downloaded archive, pipe reader failed, %w", err)
		if err := pr.CloseWithError(err); err != nil {
			level.Error(r.logger).Log("msg", "pr close", "err", err)
		}

		return err
	}

	err = writeCacheMetadata(CacheMetadata{CacheSizeBytes: uint64(written), Dstpath: dst}, cacheFileName)
	if err != nil {
		level.Error(r.logger).Log("msg", "writeCacheMetadata", "err", err)
	}

	level.Info(r.logger).Log("msg", "downloaded to local", "directory", dst, "cache size", humanize.Bytes(uint64(written)))

	level.Debug(r.logger).Log(
		"msg", "archive extracted",
		"local", dst,
		"remote", src,
		"raw size", written,
	)

	return nil
}

// Helpers

func (r restorer) generateKey(parts ...string) (string, error) {
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

func getSeparator() string {
	if runtime.GOOS == "windows" {
		return `\`
	}

	return "/"
}

func writeCacheMetadata(data CacheMetadata, filename string) error {
	// Lock the mutex to prevent concurrent writes
	cacheFileMutex.Lock()
	defer cacheFileMutex.Unlock()

	// Ensure the directory exists
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Initialize a slice to hold the cache metadata
	var cacheData []CacheMetadata

	// Check if the file exists
	if _, err := os.Stat(filename); err == nil {
		// File exists, read and unmarshal its contents
		fileContent, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read cache file %s: %w", filename, err)
		}

		// Unmarshal the JSON into the slice
		if len(fileContent) > 0 {
			err = json.Unmarshal(fileContent, &cacheData)
			if err != nil {
				return fmt.Errorf("failed to unmarshal existing cache data: %w", err)
			}
		}
	}

	// Append the new metadata to the slice
	cacheData = append(cacheData, data)

	// Marshal the updated slice back to JSON
	updatedData, err := json.MarshalIndent(cacheData, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal updated cache data: %w", err)
	}

	// Write the updated JSON to the file
	err = os.WriteFile(filename, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated cache data to file %s: %w", filename, err)
	}

	fmt.Println("Successfully updated cache metrics to", filename)
	return nil
}
