package cache

import "github.com/meltwater/drone-cache/key"

type options struct {
	namespace                  string
	fallbackGenerator          key.Generator
	override                   bool
	failRestoreIfKeyNotPresent bool
	gracefulDetect             bool
	ignoreMissingPaths         bool
	enableCacheKeySeparator    bool
	strictKeyMatching          bool
	cacheType                  string
}

// Option overrides behavior of Archive.
type Option interface {
	apply(*options)
}

type optionFunc func(*options)

func (f optionFunc) apply(o *options) {
	f(o)
}

// WithNamespace sets namespace option.
func WithNamespace(s string) Option {
	return optionFunc(func(o *options) {
		o.namespace = s
	})
}

// WithFallbackGenerator sets fallback key generator option.
func WithFallbackGenerator(g key.Generator) Option {
	return optionFunc(func(o *options) {
		o.fallbackGenerator = g
	})
}

// WithOverride sets object should be overriten even if it already exists.
func WithOverride(override bool) Option {
	return optionFunc(func(o *options) {
		o.override = override
	})
}

// WithGracefulDetect sets option used by automatic cache detection to skip
// missing directories without failing the rebuild when nothing was uploaded.
func WithGracefulDetect(gracefulDetect bool) Option {
	return optionFunc(func(o *options) {
		o.gracefulDetect = gracefulDetect
	})
}

// WithIgnoreMissingPaths skips missing source paths during Save Cache while
// still requiring at least one existing path to be cached.
func WithIgnoreMissingPaths(ignoreMissingPaths bool) Option {
	return optionFunc(func(o *options) {
		o.ignoreMissingPaths = ignoreMissingPaths
	})
}

// WithFailRestoreIfKeyNotPresent sets option to fail restore if key does not exist.
func WithFailRestoreIfKeyNotPresent(b bool) Option {
	return optionFunc(func(o *options) {
		o.failRestoreIfKeyNotPresent = b
	})
}

// WithEnableCacheKeySeparator controls whether a separator is used when structuring cache paths.
func WithEnableCacheKeySeparator(enableCacheKeySeparator bool) Option {
	return optionFunc(func(o *options) {
		o.enableCacheKeySeparator = enableCacheKeySeparator
	})
}

// WithStrictKeyMatching controls whether cache keys must match exactly.
// When true, prevents unintended cache restoration with similar keys (e.g., "key" vs "key1").
func WithStrictKeyMatching(strictKeyMatching bool) Option {
	return optionFunc(func(o *options) {
		o.strictKeyMatching = strictKeyMatching
	})
}

// WithCacheType threads the cache type into cache components (used to select unified vs legacy behavior).
func WithCacheType(cacheType string) Option {
	return optionFunc(func(o *options) {
		o.cacheType = cacheType
	})
}
