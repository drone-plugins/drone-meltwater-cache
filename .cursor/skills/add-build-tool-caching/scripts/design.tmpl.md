# Design: {{TOOL}}

<!-- Replace every TODO, then re-run `cache-intel.sh design`. The Summary, Limitations and
Compatibility sections are copied into REPORT.md and the pull request description. -->

## Summary
TODO: two to four sentences for reviewers: what is detected, what is cached, the key, and what build steps (or harness-core) must provide.

## Mapping entry
TODO: globToDetect, tool id, preparer, additionalCacheDirs, position in buildToolInfoMapping and why, and the effect on composite keys of mixed repositories.

## Paths
| Path | Type | Strategy | Reason |
|------|------|----------|--------|
| TODO | | | |

## Scenario matrix
| Scenario | Detected tools | Cached paths | Key inputs | Covered by |
|----------|----------------|--------------|------------|------------|
| single project at root | TODO | | | |
| project one level deep | TODO | | | |
| explicit env override | TODO | | | |
| lockfile changed | TODO | | | |
| mixed with an existing tool | TODO | | | |
| PLUGIN_MOUNT or custom key | TODO | | | |

## Compatibility and migration
TODO: effect on existing keys and paths, and the expected first cold run after rollout.

## Limitations
TODO: unsupported layouts, platforms, versions, and anything deliberately left out.
