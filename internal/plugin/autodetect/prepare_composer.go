package autodetect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	composerJSONFileName = "composer.json"
	composerVendorDir    = "vendor"
)

type composerConfig struct {
	Config struct {
		VendorDir string `json:"vendor-dir"`
		CacheDir  string `json:"cache-dir"`
	} `json:"config"`
}

type composerPreparer struct{}

func newComposerPreparer() *composerPreparer {
	return &composerPreparer{}
}

func (*composerPreparer) PrepareRepo(dir string) (string, error) {
	return prepareComposer(dir)
}

type composerFallbackPreparer struct{}

func newComposerFallbackPreparer() *composerFallbackPreparer {
	return &composerFallbackPreparer{}
}

func (*composerFallbackPreparer) PrepareRepo(dir string) (string, error) {
	return prepareComposer(dir)
}

// prepareComposer resolves the vendor directory for Composer.
// Precedence:
// 1. COMPOSER_VENDOR_DIR environment variable
// 2. config.vendor-dir in composer.json
// 3. Default "vendor" inside dir
func prepareComposer(dir string) (string, error) {
	if envVendor := strings.TrimSpace(os.Getenv("COMPOSER_VENDOR_DIR")); envVendor != "" {
		return resolveRepoPath(dir, envVendor)
	}

	cfg, err := readComposerConfig(filepath.Join(dir, composerJSONFileName))
	if err != nil {
		return "", err
	}

	if cfg != nil && strings.TrimSpace(cfg.Config.VendorDir) != "" {
		return resolveRepoPath(dir, strings.TrimSpace(cfg.Config.VendorDir))
	}

	return filepath.Join(dir, composerVendorDir), nil
}

// composerCacheDirs resolves the Composer package download cache directory when
// explicitly configured via COMPOSER_CACHE_DIR, config.cache-dir in composer.json,
// or when the default cache directory already resides on the shared workspace.
func composerCacheDirs(dir string) ([]string, error) {
	cacheDir, err := effectiveComposerCacheDir(dir)
	if err != nil || cacheDir == "" {
		return nil, err
	}

	return []string{cacheDir}, nil
}

func effectiveComposerCacheDir(dir string) (string, error) {
	if envCache := strings.TrimSpace(os.Getenv("COMPOSER_CACHE_DIR")); envCache != "" {
		return resolveRepoPath(dir, envCache)
	}

	cfg, err := readComposerConfig(filepath.Join(dir, composerJSONFileName))
	if err != nil {
		return "", err
	}

	if cfg != nil && strings.TrimSpace(cfg.Config.CacheDir) != "" {
		return resolveRepoPath(dir, strings.TrimSpace(cfg.Config.CacheDir))
	}

	// Check if standard cache locations reside on the shared workspace.
	// We check XDG_CACHE_HOME, then UserHomeDir.
	root := workspaceRoot(dir)
	if root == "" {
		return "", nil
	}

	if xdgCache := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdgCache != "" {
		defaultCache := filepath.Clean(filepath.Join(xdgCache, "composer"))
		if pathWithin(defaultCache, root) {
			return defaultCache, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", nil
	}

	candidates := []string{
		filepath.Join(home, ".cache", "composer"),
		filepath.Join(home, ".composer", "cache"),
	}

	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if pathWithin(cleaned, root) {
			return cleaned, nil
		}
	}

	return "", nil
}

func readComposerConfig(path string) (*composerConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var cfg composerConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return &cfg, nil
		}
		return nil, fmt.Errorf("failed to decode %s: %w", path, err)
	}

	return &cfg, nil
}
