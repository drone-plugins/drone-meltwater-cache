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

// Restore metric constants
const (
	metricKeyCacheHit   = "cache_hit"
	metricKeyCacheState = "cache_state"
	metricValueTrue     = "true"
	metricValueFalse    = "false"
	metricValueComplete = "complete"
	metricValuePartial  = "partial"
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
	strictKeyMatching       bool
	backend                 string
	accountID               string
	cacheType               string
}

var cacheFileMutex sync.Mutex // To ensure thread-safe writes to the file

// NewRestorer creates a new cache.Restorer.
func NewRestorer(logger log.Logger, s storage.Storage, a archive.Archive, g key.Generator, fg key.Generator, namespace string, failIfKeyNotPresent bool, enableCacheKeySeparator bool, strictKeyMatching bool, backend, accountID string, cacheType string) Restorer { // nolint:lll
	return restorer{
		logger:                  logger,
		a:                       a,
		s:                       s,
		g:                       g,
		fg:                      fg,
		namespace:               namespace,
		failIfKeyNotPresent:     failIfKeyNotPresent,
		enableCacheKeySeparator: enableCacheKeySeparator,
		strictKeyMatching:       strictKeyMatching,
		backend:                 backend,
		accountID:               accountID,
		cacheType:               cacheType,
	}
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
		wg               sync.WaitGroup
		errs             = &internal.MultiError{}
		namespace        = filepath.ToSlash(filepath.Clean(r.namespace))
		successCount     int // Counter to track how many directories were successfully restored
		totalDirectories int // Counter for total directories to process
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
				// Only prepend legacy intel prefix when unified cache type is not enabled
				if strings.TrimSpace(r.cacheType) == "" {
					prefix = r.accountID + "/intel/" + prefix
				}
			}

			for _, e := range entries {
				entryPath := e.Path
				var dst string

				level.Info(r.logger).Log("msg", "processing entry", "entryPath", entryPath, "prefix", prefix, "key", key)

				// Check if we're in strict matching mode and skip entries that don't exactly match the key pattern
				// In strict mode: we only want to match entries where:
				// 1. The entry path exactly equals the prefix, OR
				// 2. The entry path starts with the prefix + separator, OR
				// 3. A path component exactly matches the key
				if r.strictKeyMatching {
					// REAL WORLD KEY PATTERN CHECK - Check for when key is a substring at the beginning
					// Example: key="keypattern", entry="keypattern1/path"
					pathParts := strings.Split(entryPath, getSeparator())
					firstComponent := ""
					if len(pathParts) > 0 {
						firstComponent = pathParts[0]
					}

					// Check for key + number pattern (e.g., "keypattern1")
					if strings.HasPrefix(firstComponent, key) && len(firstComponent) > len(key) {
						// The component starts with our key but has additional characters
						extraPart := firstComponent[len(key):]
						level.Debug(r.logger).Log("msg", "detected key with suffix", "component", firstComponent,
							"key", key, "extra", extraPart)

						// Only allow if it's exactly the key (no suffix)
						level.Debug(r.logger).Log("msg", "skipping entry with key suffix in strict mode",
							"entryPath", entryPath, "key", key, "component", firstComponent)
						continue
					}

					// Standard matching cases
					if entryPath == prefix {
						// Case 1: Exact match with the prefix
						// Example: key="pattern", prefix="repo/pattern", entryPath="repo/pattern"
						level.Debug(r.logger).Log("msg", "strict match: exact prefix", "entryPath", entryPath, "prefix", prefix)
					} else if strings.HasPrefix(entryPath, prefix+getSeparator()) {
						// Case 2: Entry starts with prefix + separator
						// Example: key="pattern", prefix="repo/pattern", entryPath="repo/pattern/path1"
						level.Debug(r.logger).Log("msg", "strict match: path starts with prefix+separator", "entryPath", entryPath, "prefix", prefix)
					} else {
						// Case 3: Check if any path component exactly matches the key
						pathComponents := strings.Split(entryPath, getSeparator())
						exactMatch := false

						for _, component := range pathComponents {
							if component == key {
								exactMatch = true
								break
							}
						}

						if !exactMatch {
							level.Debug(r.logger).Log("msg", "skipping non-exact match in strict mode",
								"entryPath", entryPath, "key", key, "prefix", prefix)
							continue
						} else {
							level.Debug(r.logger).Log("msg", "strict match: component exactly matches key", "entryPath", entryPath, "key", key)
						}
					}
				}

				remotePath := entryPath
				level.Debug(r.logger).Log("msg", "original remote path", "remotePath", remotePath, "strictMode", r.strictKeyMatching)

				// Initial path extraction based on separator settings
				if r.enableCacheKeySeparator {
					dst = strings.TrimPrefix(entryPath, prefix)
				} else {
					dst = strings.TrimPrefix(entryPath, prefix+getSeparator())
				}

				level.Debug(r.logger).Log("msg", "initial path trim", "dst", dst, "entryPath", entryPath, "prefix", prefix)

				if strings.HasPrefix(dst, namespace) || strings.Contains(dst, key) {
					level.Debug(r.logger).Log("msg", "path needs special processing", "dst", dst, "keyInPath", strings.Contains(dst, key))

					// Extract path components for more precise processing
					pathComponents := strings.Split(entryPath, getSeparator())
					level.Debug(r.logger).Log("msg", "path components", "components", fmt.Sprintf("%v", pathComponents))

					// Find the component containing our key
					keyComponentIndex := -1
					for i, component := range pathComponents {
						if strings.Contains(component, key) {
							keyComponentIndex = i
							level.Debug(r.logger).Log("msg", "found key in component", "index", i, "component", component)
							break
						}
					}

					if keyComponentIndex >= 0 && keyComponentIndex+1 < len(pathComponents) {
						pathOnly := strings.Join(pathComponents[keyComponentIndex+1:], getSeparator())
						level.Debug(r.logger).Log("msg", "extracted path after key component", "pathOnly", pathOnly, "keyComponent", pathComponents[keyComponentIndex])
						dst = pathOnly
					} else if keyComponentIndex >= 0 && !r.strictKeyMatching {
						// In flexible mode, we should still process entries even if they don't have components after the key
						// This is especially important for entries like "keypattern1" when searching for "keypattern"
						level.Debug(r.logger).Log("msg", "flexible mode - using key component as path", "component", pathComponents[keyComponentIndex])

						// For entries that match the key pattern directly but have no path after,
						// extract the unique part to be used as the destination
						component := pathComponents[keyComponentIndex]
						if strings.HasPrefix(component, key) && len(component) > len(key) {
							// Use the component itself as the path
							dst = component
						} else if len(pathComponents) > keyComponentIndex {
							// If there are no more components, use the key component as is
							dst = component
						}
					}
				}

				if dst != "" {
					// Always use the actual remotePath for source path mapping to ensure correct file retrieval
					level.Debug(r.logger).Log("msg", "adding destination", "dst", dst, "sourcePath", remotePath)
					dsts = append(dsts, dst)
					sourcePaths[dst] = remotePath
				} else {
					level.Debug(r.logger).Log("msg", "skipping empty destination", "entryPath", entryPath)
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
			} else {
				// Increment success counter if directory is successfully restored
				successCount++
			}
		}(src, dst)
	}

	wg.Wait()

	totalDirectories = len(dsts)

	metrics := make(map[string]string)
	// If at least one directory was successfully restored, consider the operation successful
	metrics[metricKeyCacheHit] = metricValueFalse
	if successCount > 0 {
		metrics[metricKeyCacheHit] = metricValueTrue
		if successCount == totalDirectories {
			metrics[metricKeyCacheState] = metricValueComplete
			level.Info(r.logger).Log("msg", "cache restored", "took", time.Since(now), "status", "all directories successfully restored")
		} else {
			metrics[metricKeyCacheState] = metricValuePartial
			level.Info(r.logger).Log("msg", "cache restored", "took", time.Since(now),
				"status", fmt.Sprintf("partially restored (%d/%d directories)", successCount, totalDirectories))
		}
		r.exportMetricsIfUnified(metrics)
		return nil
	}
	r.exportMetricsIfUnified(metrics)
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
		err = fmt.Errorf("failed to extract files from downloaded archive, %w", err)
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

// exportMetricsIfUnified exports metrics only for unified cache flow
func (r restorer) exportMetricsIfUnified(metrics map[string]string) {
	// Only export metrics for unified cache flow
	if strings.TrimSpace(r.cacheType) == "" {
		return
	}

	if err := exportRestoreMetrics(r.logger, metrics); err != nil {
		level.Error(r.logger).Log("msg", "failed to export restore metrics", "err", err)
	}
}

func exportRestoreMetrics(logger log.Logger, outputs map[string]string) error {
	// Check if DRONE_OUTPUT environment variable is set
	outputPath := os.Getenv("DRONE_OUTPUT")
	if outputPath == "" {
		return fmt.Errorf("DRONE_OUTPUT environment variable is not set")
	}

	// Open the file with append mode
	outputFile, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open output file %s: %w", outputPath, err)
	}
	defer outputFile.Close()

	// Write all key-value pairs
	for key, value := range outputs {
		if key == "" {
			continue // Skip empty keys
		}
		_, err = fmt.Fprintf(outputFile, "%s=%s\n", key, value)
		if err != nil {
			// Log error but continue with next key
			level.Error(logger).Log("msg", "failed to export metric", "key", key, "err", err)
			continue
		}
	}

	return nil
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
