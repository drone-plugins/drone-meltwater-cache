# Cache Intelligence: cargo

## Summary

Full autodetect caching support for **Cargo (Rust)** is now implemented in
drone-cache. Triage classification: **new tool within engine capabilities**
(root/one-level detection, download-cache paths, lockfile-based key) — the full
procedure was run. The plugin detects `Cargo.lock` (primary) or `Cargo.toml`
(lockless fallback), caches Cargo's package download and index caches under the
resolved `CARGO_HOME`, and writes no configuration into the repository. Unit and
scenario tests pass, `go vet` is clean, Linux/Windows cross-compiles succeed, and
the full local restore → install → save → clean → restore → offline-install →
invalidation lifecycle was validated against a real Rust fixture with the actual
`cargo` toolchain. Remote QA0 validation is `REMOTE_BLOCKED` (no cargo candidate
pipeline; candidate image publication not authorized). Overall local terminal
state: **LOCAL_COMPLETE**.

## Revisions

- Starting: `feat/php-caching-support` @ `852812f6e83d3aadf193fe4e767d7e431daf1a41`
- Ending: `feat/cargo-caching-support` @ (this commit)
- Candidate binary: `bin/drone-cache` (local build), release targets
  `release/linux/amd64/drone-cache`, `release/windows/amd64/drone-cache.exe`
- Candidate image: not built/published (publication not authorized)
- Baseline image: n/a

## Behavior

- **Detection files and precedence**: `Cargo.lock` first (mapping entry id
  `cargo`), then `Cargo.toml` as a fallback that is skipped when `Cargo.lock`
  exists in the same directory (`excludeIfExist: [Cargo.lock]`). Both entries
  share the `cargo` tool identifier, so only one is ever selected. Discovery is
  root then one directory level, per engine capability.
- **Cached paths**, strategy **follow-only** (no config written), cache type
  **download cache / index / metadata**, resolved under `CARGO_HOME`:
  - `registry/cache` — downloaded `.crate` tarballs (primary, returned by
    `PrepareRepo`)
  - `registry/index` — crates.io registry index (additional)
  - `git/db` — bare clones of git dependencies (additional)
- **Key inputs**: MD5 digest of `Cargo.lock` (or `Cargo.toml` for the lockless
  fallback). 32-char lowercase hex, unchanged legacy key algorithm.
- **Configuration precedence**: `CARGO_HOME` env var (followed as-is; relative
  values resolve against the project dir) → repo-local `.cache/cargo` default.
- **Files written to the repository**: none. Cargo has no project-local config
  file that can relocate `CARGO_HOME` (it is read at process start), so the plugin
  does not attempt to write one.
- **Wiring required in later build steps**: for the repo-local default to be
  consumed, later steps must export `CARGO_HOME=.cache/cargo` (relative to the
  project directory). When the build already exports `CARGO_HOME`, no wiring is
  needed and that path is cached as-is.
- **Deliberately excluded paths**: `registry/src` and `git/checkouts`
  (re-extractable from the download caches), `target/` (toolchain/ABI-coupled,
  large build output), installed binaries under `bin/`, and
  `config.toml` / `credentials.toml` (may contain registry tokens).

## Evidence

| Check | Result | Evidence |
|-------|--------|----------|
| Unit / scenario tests | PASS | `go test -count=1 ./internal/plugin/autodetect ./internal/plugin` → both `ok`. 7 unit + 10 scenario cargo tests. |
| Full test, vet, build | PASS (code) / env-blocked (backends) | `go test ./...`: all packages pass except `storage/backend/gcs` and `storage/backend/s3`, which fail only on `connect: permission denied` (sandbox blocks localhost sockets) — pre-existing, unrelated to this change. `go vet ./...` clean. `go build ./...` and both cross-compiles succeed. |
| Cross-compilation | PASS | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` and `GOOS=windows GOARCH=amd64` builds OK; local `bin/drone-cache` OK. |
| Local cold restore | PASS | Graceful miss, exit 0. Key `b87b85fd762192dcd5d7058d0500f16b`. Three cargo dirs targeted; absent paths non-fatal. |
| Local cold install | PASS | `cargo fetch` populated `registry/cache` (`.crate` tarballs) and `registry/index`; `COLD_FETCH_MS≈124`. |
| Local save | PASS | 2 uploaded (`registry/cache` 53kB→49kB, `registry/index` 42kB→34kB), `git/db` skipped missing, 0 failed; `took=740µs`. |
| Local warm restore | PASS | Exit 0, "partially restored (2/3 directories)"; crates + index restored to `.cache/cargo`; `took=890µs`. git/db legitimately absent. |
| Package manager consumed restore | PASS | `cargo fetch --offline` after warm restore → RC 0 in 38ms, using only restored data (no network). |
| Local invalidation | PASS | Editing `Cargo.lock` changed the key `b87b85fd…` → `d83d10d1…`; restore fell back to a graceful cold miss. |
| Harness cold / warm / invalidation | BLOCKED | No cargo candidate-validation pipeline in `default/qajt`; the only cargo-defaulted pipeline (`cache_intel_worker_pipeline`) is an agent orchestrator, and candidate image publication is not authorized. |
| Compatibility (legacy keys, archives, overrides) | PASS | New mapping entries append after `golang`; existing tools' keys/paths unchanged. Hybrid Rust+Node scenario test confirms both managers detected with stable order. Key shape is 32-char lowercase hex (legacy algorithm preserved). |

## Evidence provenance

- **Documented** (Cargo book, targeted at Cargo 1.x): `CARGO_HOME` layout
  (`registry/cache`, `registry/index`, `registry/src`, `git/db`,
  `git/checkouts`, `bin`, `config.toml`, `credentials.toml`); `CARGO_HOME` read
  at process start; `.crate` tarballs validated against `Cargo.lock` checksums.
- **Observed** (cargo 1.99.0 / rustc 1.99.0, linux/amd64, rustup minimal
  toolchain): cold/warm fetch behavior, offline consumption, cache directory
  contents, archive sizes, key invalidation — see Evidence table.
- **Assumptions**: Windows path behavior validated only via cross-compilation and
  `filepath` usage, not a native Windows run (confidence: medium). macOS not
  tested.

## Command ledger

- PASS: `go test -count=1 ./internal/plugin/autodetect ./internal/plugin`;
  `go vet ./...`; `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`;
  `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build`; `go build -o bin/drone-cache .`;
  local lifecycle (cold restore, cold fetch, save, warm restore, offline fetch,
  invalidation).
- FAIL: none attributable to this change.
- SKIPPED / BLOCKED / NOT_APPLICABLE:
  - `go test ./...` backend suites `gcs`, `s3` — environment-blocked
    (`connect: permission denied`, sandbox has no localhost sockets); unrelated.
  - Harness QA0 remote cold/warm/invalidation — BLOCKED. Owner/action: a
    maintainer must (a) build and publish a candidate plugin image from this
    branch, and (b) provide or point to a cargo cache-sanity pipeline in
    `default/qajt` (analogous to `ci_composer_cache_v1`) that references the
    candidate image. Then run cold/warm/invalidation.

## Measurements

- Cold vs. warm install: `cargo fetch` cold ≈124ms (network) vs. `cargo fetch
  --offline` warm ≈38ms (restored cache, no network). Representative only — this
  is a tiny two-crate fixture; real crate graphs save far more.
- Archive size (compressed): `registry/cache` 49kB, `registry/index` 34kB.
- Restore/save overhead: restore `took≈890µs`, save `took≈740µs` for this fixture.

## Compatibility and migration

No existing tool's key or path changes: the two new entries are appended after
the `golang` entry and only match `Cargo.lock` / `Cargo.toml`, which no prior
entry matched. Repositories that already use Cargo will see one cold run the first
time cargo caching activates (expected, since there was no prior cargo cache),
then warm thereafter. No migration of existing keys.

## Limitations and open decisions

- Only Cargo's download/index caches are cached; `target/` build artifacts are
  intentionally not cached (ABI/toolchain coupling, size).
- Nested detection is limited to one directory level (engine capability). Cargo
  workspaces whose member crates live two or more levels below the root are not
  individually detected; the root `Cargo.lock`/`Cargo.toml` still drives caching.
- The repo-local default requires later steps to export `CARGO_HOME=.cache/cargo`
  to be consumed; when the build already sets `CARGO_HOME`, no wiring is needed.
- Native Windows/macOS behavior is not directly validated (Linux only here).

## Feedback incorporated

- The `git/db` path is legitimately absent for crates without git dependencies;
  the lifecycle confirmed graceful "2/3 directories" partial restore and a
  save-time skip rather than a failure, matching the engine's graceful-detect
  contract for auto-detected paths. No code change needed — captured as expected
  behavior and asserted indirectly by the save/warm-restore evidence.

## Rollout and rollback

- Rollout: additive. Ship the binary/image; cargo repos get a cold-then-warm
  cycle on first activation.
- Rollback: reverting the two `buildToolInfoMapping` entries and
  `prepare_cargo.go` fully disables cargo detection with no effect on other
  tools' keys or paths. No stored-cache migration concerns because no existing
  key changes.
