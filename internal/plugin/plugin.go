// Package plugin for caching directories using given backends
package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meltwater/drone-cache/internal/plugin/autodetect"

	"github.com/meltwater/drone-cache/archive"
	"github.com/meltwater/drone-cache/cache"
	"github.com/meltwater/drone-cache/internal/metadata"
	"github.com/meltwater/drone-cache/key"
	keygen "github.com/meltwater/drone-cache/key/generator"
	"github.com/meltwater/drone-cache/storage"
	"github.com/meltwater/drone-cache/storage/backend"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// Error recognized error from plugin.
type Error string

// Error is a sentinel plugin error.
func (e Error) Error() string { return string(e) }

// Unwrap unwraps underlying error.
func (e Error) Unwrap() error { return e }

// Plugin stores metadata about current plugin.
type Plugin struct {
	logger log.Logger

	Metadata metadata.Metadata
	Config   Config
}

// New creates a new plugin.
func New(logger log.Logger) *Plugin {
	return &Plugin{logger: logger}
}

func autoCacheKey(accountID, manifestHash string) string {
	return fmt.Sprintf("%s/%s", accountID, manifestHash)
}

// Exec entry point of Plugin, where the magic happens.
func (p *Plugin) Exec() error { // nolint:funlen
	cfg := p.Config

	// 1. Check parameters
	if cfg.Debug {
		level.Debug(p.logger).Log("msg", "DEBUG MODE enabled!")

		for _, pair := range os.Environ() {
			level.Debug(p.logger).Log("var", pair)
		}

		level.Debug(p.logger).Log("msg", "plugin initialized wth config", "config", fmt.Sprintf("%#v", p.Config))
		level.Debug(p.logger).Log("msg", "plugin initialized with metadata", "metadata", fmt.Sprintf("%#v", p.Metadata))
	}

	// FLUSH

	if cfg.Rebuild && cfg.Restore {
		return errors.New("rebuild and restore are mutually exclusive, please set only one of them")
	}

	var localRoot string
	if p.Config.LocalRoot != "" {
		localRoot = filepath.Clean(p.Config.LocalRoot)
	} else {
		workspace, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory, %w", err)
		}

		localRoot = workspace
	}

	var options []cache.Option
	// Removing below namespace from cache path so that saved cache path is same for PR and manual runs.
	// if p.Config.RemoteRoot != "" {
	// 	options = append(options, cache.WithNamespace(p.Config.RemoteRoot))
	// } else {
	// 	options = append(options, cache.WithNamespace(p.Metadata.Repo.Name))
	// }

	var generator key.Generator

	switch {
	case cfg.AutoDetect:
		{
			var toolDetected, keyOverriden bool = false, false
			pathOverridden := len(p.Config.Mount) > 0
			dirs, buildTools, cacheKey, err := autodetect.DetectDirectoriesToCache(pathOverridden)
			if err != nil {
				return fmt.Errorf("autodetect enabled but failed to detect, falling back to default, %w", err)
			}
			if err := autodetect.ValidateDetectedPaths(dirs); err != nil {
				return err
			}
			if len(buildTools) > 0 {
				toolDetected = true
				p.logger.Log("msg", "build tools detected: "+strings.Join(buildTools, ", ")) //nolint: errcheck
			} else if pathOverridden {
				p.logger.Log("msg", "using provided cache path") //nolint: errcheck
			} else {
				p.logger.Log("msg", "no supported build tool detected") //nolint: errcheck
			}
			if cfg.CacheKeyTemplate != "" {
				keyOverriden = true
				cacheKey = cfg.CacheKeyTemplate
			}

			// Detection reads the workspace, and the build changes the workspace
			// between restore and save: `npm install` writes a package-lock.json
			// that restore never saw, which changes the hashed manifest and the
			// cacheable paths. The remote object name embeds both, so save
			// replays the plan restore recorded instead of re-deriving it.
			//
			// A custom key or a custom path means autodetection is not deciding
			// anything, so there is nothing to hand over.
			if !keyOverriden && !pathOverridden {
				if cfg.Restore && toolDetected {
					sources, err := autodetect.DetectNpmPackageJSONSources()
					if err != nil {
						return fmt.Errorf("identify autodetected cache sources: %w", err)
					}
					plan := autodetect.AutoDetectPlan{Key: cacheKey, Sources: sources}
					if err := autodetect.WriteAutoDetectPlan(plan); err != nil {
						level.Warn(p.logger).Log("msg",
							"could not record the autodetected cache plan; the save step will skip safely",
							"err", err)
					}
				}

				if cfg.Rebuild {
					plan, found, err := autodetect.ReadAutoDetectPlan()
					switch {
					case err != nil:
						level.Warn(p.logger).Log("msg",
							"could not read the cache plan recorded by the restore step; skipping cache save",
							"err", err)
						_ = autodetect.RemoveAutoDetectPlan()
						return nil
					case found:
						cacheKey = plan.Key
						dirs = autodetect.FilterPathsForPlan(dirs, plan.Sources)
						toolDetected = true
					default:
						level.Warn(p.logger).Log("msg",
							"no cache plan recorded by the restore step; skipping cache save")
						return nil
					}
				}
			}

			if !pathOverridden {
				p.Config.Mount = dirs
				options = append(options, cache.WithGracefulDetect(true))
			} else {
				options = append(options, cache.WithGracefulDetect(false))
			}

			/*
				Tool Detected    Key Override    Path Override    Key Used      Path Used
				---------------------------------------------------------------
				Yes              Yes             Yes              user key      user path
				Yes              Yes             No               user key      auto path
				Yes              No              Yes              do nothing    do nothing
				Yes              No              No               auto key      auto path
				No               Yes             Yes              user key      user path
				No               Yes             No               do nothing    do nothing
				No               No              Yes              do nothing    do nothing
				No               No              No               do nothing    do nothing
			*/
			if pathOverridden && !keyOverriden {
				p.logger.Log("msg", "A key must be provided if any custom paths are used. Skipping cache")
				return nil
			}

			if !toolDetected {
				if !keyOverriden && !pathOverridden {
					p.logger.Log("msg", "no safe automatic cache directories detected")
					return nil
				}
			}

			if cacheKey == "" {
				cacheKey = "default"
			}

			generator = keygen.NewMetadata(p.logger, autoCacheKey(cfg.AccountID, cacheKey), p.Metadata)
			if err := generator.Check(); err != nil {
				return fmt.Errorf("parse failed, falling back to default, %w", err)
			}

			options = append(options, cache.WithFallbackGenerator(keygen.NewHash(cfg.AccountID+p.Metadata.Commit.Branch)))
		}
	case cfg.CacheKeyTemplate != "":
		generator = keygen.NewMetadata(p.logger, cfg.CacheKeyTemplate, p.Metadata)
		if err := generator.Check(); err != nil {
			return fmt.Errorf("parse failed, falling back to default, %w", err)
		}

		options = append(options, cache.WithFallbackGenerator(keygen.NewHash(p.Metadata.Commit.Branch)))
	default:
		{
			generator = keygen.NewHash(p.Metadata.Commit.Branch)
			options = append(options, cache.WithFallbackGenerator(keygen.NewStatic(p.Metadata.Commit.Branch)))
		}
	}

	options = append(options, cache.WithOverride(p.Config.Override),
		cache.WithFailRestoreIfKeyNotPresent(p.Config.FailRestoreIfKeyNotPresent),
		cache.WithEnableCacheKeySeparator(p.Config.EnableCacheKeySeparator),
		cache.WithStrictKeyMatching(p.Config.StrictKeyMatching))

	// Thread cache type from Harness backend config to cache layer to decide unified vs legacy behavior
	if cfg.Backend == backend.Harness {
		options = append(options, cache.WithCacheType(cfg.Harness.CacheType))
	}

	// 2. Initialize storage backend.
	b, err := backend.FromConfig(p.logger, cfg.Backend, backend.Config{
		Debug:      cfg.Debug,
		Azure:      cfg.Azure,
		FileSystem: cfg.FileSystem,
		GCS:        cfg.GCS,
		S3:         cfg.S3,
		SFTP:       cfg.SFTP,
		Harness:    cfg.Harness,
	})
	if err != nil {
		return fmt.Errorf("initialize backend <%s>, %w", cfg.Backend, err)
	}

	// 3. Initialize cache.
	c := cache.New(p.logger,
		storage.New(p.logger, b, cfg.StorageOperationTimeout),
		archive.FromFormat(p.logger, localRoot, cfg.ArchiveFormat,
			archive.WithSkipSymlinks(cfg.SkipSymlinks),
			archive.WithCompressionLevel(cfg.CompressionLevel),
			archive.WithPreserveMetadata(cfg.PreserveMetadata && (cfg.Backend == backend.S3 || cfg.Backend == backend.GCS || cfg.Backend == backend.Azure)),
		),
		generator,
		cfg.Backend,
		cfg.AccountID,
		options...,
	)

	// 4. Expand the mount paths.
	p.Config.Mount = expandConfigPath(p.Config.Mount)

	// 5. Select mode
	if cfg.Rebuild {
		if err := c.Rebuild(p.Config.Mount); err != nil {
			level.Debug(p.logger).Log("err", fmt.Sprintf("%+v\n", err))
			return Error(fmt.Sprintf("[IMPORTANT] build cache, %+v\n", err))
		}
		if cfg.AutoDetect && cfg.CacheKeyTemplate == "" && len(cfg.Mount) == 0 {
			if err := autodetect.RemoveAutoDetectPlan(); err != nil {
				level.Warn(p.logger).Log("msg", "could not remove consumed cache plan", "err", err)
			}
		}
	}

	if cfg.Restore {
		if err := c.Restore(p.Config.Mount, p.Config.MetricsFile); err != nil {
			level.Debug(p.logger).Log("err", fmt.Sprintf("%+v\n", err))
			return Error(fmt.Sprintf("[IMPORTANT] restore cache, %+v\n", err))
		}
	}

	// FLUSH

	return nil
}
