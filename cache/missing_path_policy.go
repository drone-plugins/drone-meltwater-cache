package cache

// MissingPathPolicy controls how Rebuild handles source paths that do not exist.
//
// Keep this separate from automatic cache detection so enabling ignore-missing
// paths does not change Cache Intelligence / auto-detect behavior.
type MissingPathPolicy int

const (
	// MissingPathStrict fails Rebuild as soon as any configured source path is missing.
	// This is the default for explicit Save Cache steps.
	MissingPathStrict MissingPathPolicy = iota

	// MissingPathSkipAllowEmpty skips missing source paths and still succeeds when
	// every path is missing. Preserves existing automatic-detection behavior.
	MissingPathSkipAllowEmpty

	// MissingPathSkipRequirePresent skips missing source paths with a warning, caches
	// remaining paths, and fails when no existing path remains to cache.
	MissingPathSkipRequirePresent
)

// resolveMissingPathPolicy selects the rebuild missing-path policy.
// IgnoreMissingPaths takes precedence when enabled so explicit Save Cache
// opt-in behavior is never overloaded by auto-detect graceful skipping.
func resolveMissingPathPolicy(ignoreMissingPaths, gracefulDetect bool) MissingPathPolicy {
	if ignoreMissingPaths {
		return MissingPathSkipRequirePresent
	}
	if gracefulDetect {
		return MissingPathSkipAllowEmpty
	}
	return MissingPathStrict
}
