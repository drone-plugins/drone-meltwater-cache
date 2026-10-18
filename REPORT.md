# Cache Intelligence: cargo

## Summary
Adds autodetect caching support for **Cargo (Rust)**. Triage classification:
**new tool within engine capabilities** (root/one-level `Cargo.lock` detection, a
primary cache path plus extra paths, lockfile-based key). Overall validation
status: **LOCAL_COMPLETE** — focused unit/scenario tests, full autodetect/plugin
tests, `go vet`, cross-compiled release build, and a manually executed local
restore/install/save/wipe/restore lifecycle all pass. Remote (Harness)
validation is **PENDING** — to be run in the `remote-validate` phase against the
candidate image the pipeline builds.

## Revisions
- Starting: `feat/php-caching-support` @ `2a04bac2a63abda51c0e0f38671c2da0eda70442`
- Ending: `feat/cargo-caching-w1` @ (this commit)
- Candidate image: PENDING (built and pushed by the orchestration pipeline between phases)
- Baseline image: n/a (local run)

## Behavior
- **Detection files and precedence**: `Cargo.lock` only, at the repository root or
  one directory level down. `Cargo.toml` without a lockfile is not detected. The
  mapping entry is placed after `golang` and before `dotnet`; it uses an
  independent tool identifier `cargo`, so it does not affect any existing tool's
  key or path order.
- **Cached paths, strategy, and cache type** (strategy = *follow only*; no config
  written):
  - `<CARGO_HOME>/registry/cache` — download cache (`.crate` tarballs); primary path.
  - `<CARGO_HOME>/registry/index` — registry index metadata.
  - `<CARGO_HOME>/git/db` — bare git-dependency repos; absent/skipped when none.
  - `<CARGO_HOME>` = `$CARGO_HOME` when the build sets it, else repo-local `.cache/cargo`.
- **Key inputs**: MD5 (32-char lowercase hex) of `Cargo.lock`. Dependency changes
  invalidate the key (verified by scenario test).
- **Configuration precedence and files written to the repository**: `CARGO_HOME`
  env var is the only control; the preparer writes **no** config file. Cargo has
  no project-local file that relocates the cache, so relocation is env-only.
- **Wiring required in later build steps**: the install/build step must export
  `CARGO_HOME=.cache/cargo` (matching the default) so Cargo reads/writes the
  cached directory. Without it Cargo uses `~/.cargo` and the cache is not consumed.
- **Deliberately excluded paths and why**:
  - `registry/src`, `git/checkouts` — re-extracted by Cargo from `registry/cache`
    and `git/db`; caching them wastes space and risks stale/absolute-path content.
  - `target/`, `bin/` — build output / ABI-coupled, large, not portable.
  - `credentials.toml` — registry tokens; never cached (security).

## Evidence
| Check | Result | Evidence |
|-------|--------|----------|
| Unit / scenario tests | PASS | `go test ./internal/plugin/autodetect -run Cargo` — 14 tests pass (6 preparer unit + 8 scenario) |
| Full test, vet, build | PASS | `cache-intel.sh test --local-test`: `go test ./internal/plugin/autodetect ./internal/plugin` ok; `go vet ./...` clean; `cache-intel.sh build` cross-compiled `release/linux/amd64/drone-cache` |
| Compatibility (scenario) | PASS | `cache-intel.sh test --compatibility` — scenario suite ok |
| Local cold / warm / invalidation | PASS | Manual lifecycle (below): cold miss non-fatal (exit 0); save uploaded exactly `registry/cache`, `registry/index`, `git/db`; warm restore `all directories successfully restored` (exit 0); `.crate` tarball restored intact; key invalidation covered by `TestScenario_Cargo_KeyInvalidation` |
| Archive excludes credentials / src | PASS | Save uploaded only the 3 selected paths; `registry/src` and `credentials.toml` absent from all archives (`tar tzf` grep for `cred` found nothing) |
| Package manager consumed restore | PARTIAL / SIMULATED | cargo is not installable in the agent container; install was simulated by materializing Cargo's real `CARGO_HOME` layout. Native `cargo fetch --locked` consumption is to be proven in the remote phase. |
| Harness cold / warm / invalidation | PENDING | remote-validate phase |

## Evidence provenance
- **Documented claims** (source / version): Cargo cache layout (`registry/cache`,
  `registry/index`, `registry/src`, `git/db`, `git/checkouts`) and `CARGO_HOME`
  being the sole cache-location control — Cargo Book "Cargo Home" (stable Cargo
  1.x). `registry/src` and `git/checkouts` are re-extractable — Cargo Book.
- **Observed claims** (command / environment / artifact): detection, path
  selection, env override, idempotency, no-config-file, key invalidation — Go
  tests on linux/amd64, go1.26. Lifecycle restore/save/restore — plugin binary
  against a filesystem backend on linux/amd64 (logs captured in this session).
- **Remaining assumptions**: native `cargo fetch --locked` reading the restored
  `registry/cache`/`registry/index` to install offline is assumed from
  documentation; confidence medium — to be observed in the remote phase with a
  real `rust` image.

## Command ledger
- PASS: `research`, `design`, `create`, `test --local-test`, `test --compatibility`,
  `build`, `finalize`; manual local lifecycle (cold/install/save/wipe/warm).
- SKIPPED: `cache-intel.sh test --lifecycle` — requires `cargo` on PATH, which
  cannot be installed in this container. Lifecycle was instead run manually per
  ENGINE.md (commands and results recorded in this report).
- PENDING: `remote-setup`, `remote-test` (cold + warm), `validate --env qa0` —
  remote-validate phase, after the pipeline builds the candidate image.

## Measurements
- Cold vs warm install time: not meaningfully measurable locally (install
  simulated). To be captured from the remote `install` step timing.
- Archive size / overhead (local fixture, filesystem backend): `registry/cache`
  3.1 kB uncompressed → 31 B compressed; `registry/index` 3.6 kB → 8 B; `git/db`
  1.5 kB (empty). Save of all 3 paths took ~645 µs; warm restore ~708 µs. These
  are fixture-scale only; real repos are dominated by `.crate` tarball volume.

## Compatibility and migration
No existing tool's key, path order, or detection changes: `cargo` is a new,
independent mapping entry with its own tool identifier, added after `golang`.
Existing repositories without `Cargo.lock` are unaffected. First run on a Rust
repo after rollout is a normal cold start.

## Local lifecycle proof (manual, cargo not installable in container)
Executed per ENGINE.md against `test/fixtures/cargo-simple` with the plugin
binary and a disposable filesystem backend (`CARGO_HOME` unset → repo-local
`.cache/cargo`):
1. `PLUGIN_RESTORE=true PLUGIN_REBUILD=false` → cold restore, miss reported
   non-fatally, exit 0.
2. Simulated `cargo fetch --locked` by creating the real `CARGO_HOME` layout:
   `registry/cache/.../itoa-1.0.11.crate`, `registry/index/...`, `registry/src/...`,
   `git/db`, and a `credentials.toml` (to confirm it is never cached).
3. `PLUGIN_RESTORE=false PLUGIN_REBUILD=true` → save uploaded exactly 3 archives
   (`registry/cache`, `registry/index`, `git/db`); `registry/src` and
   `credentials.toml` were not uploaded.
4. Removed `registry/cache`, `registry/index`, `git/db`, `target`.
5. `PLUGIN_RESTORE=true PLUGIN_REBUILD=false` → warm restore,
   "all directories successfully restored", exit 0; `itoa-1.0.11.crate` restored
   byte-for-byte.

## Feedback incorporated
- Renamed a test helper (`contains` → `cargoContains`) after a build collision
  with an existing `prepare_gradle_test.go` helper; added a regression-free
  compile. No behavior change.

## Remote validation plan
Exact values for the `remote-validate` phase (`cache-intel.sh remote-setup ...`):
- **Fixture repo URL**: `https://github.com/BurntSushi/ripgrep`
- **Fixture ref (pinned tag)**: `14.1.1`
- **Fixture subdir**: `.` (Cargo.lock at repo root)
- **Install image**: `rust:1-alpine`
- **Install command (with cache env wiring)**:
  `CARGO_HOME=.cache/cargo cargo fetch --locked`
- **Warm-check path (relative to fixture dir)**: `.cache/cargo/registry/cache`
  (non-empty after a warm restore proves the `.crate` download cache was restored)

Rationale: ripgrep is a small, popular, pure-Rust binary crate with a committed
`Cargo.lock` at the root and a stable tag, so `cargo fetch --locked` resolves
deterministically. `CARGO_HOME=.cache/cargo` matches the plugin's default cache
location so the fetched `.crate` tarballs land in the cached `registry/cache`.

## Rollout and rollback
Ship via a normal plugin release. Roll back by reverting the `cargo` entry in
`buildToolInfoMapping` and deleting `prepare_cargo.go`; since no existing keys
change, rollback has no cache-migration impact.
