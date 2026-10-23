# Cache Intelligence: cargo

## Summary
New tool support within the existing engine: Cargo lockfile autodetection and narrowly scoped dependency caches. Local CLI gates passed; remote validation is PENDING for the next phase. Full-suite and race environment limitations are recorded below, not treated as passes. Evidence: `cache-intel.sh create`, `test --local-test`, `test --compatibility`, `test --lifecycle`, `build`, and `finalize`, all with `--tool cargo --worker-id w2 --branch feat/php-caching-support` (exit 0).

## Revisions
- Starting: `feat/php-caching-support` @ `2a04bac2a63abda51c0e0f38671c2da0eda70442` (`git rev-parse HEAD` before implementation).
- Ending: `feat/cargo-caching-w2`; implementation commit is the commit containing this report (`git log -1` after commit).
- Candidate image / digest: PENDING, supplied by orchestration after implementation. No Docker command, push or PR was run.
- Baseline image: NOT_APPLICABLE; baseline was the source revision above.

## Behavior
Verified by `go test -count=1 ./internal/plugin/autodetect -run 'cargo|Scenario_Cargo'` and the CLI compatibility gate:
- Detect `Cargo.lock` only, root preferred, otherwise one level down; select the shortest match under existing engine semantics. No manifest fallback, recursive discovery or engine change.
- Cache `registry/cache` (download archives), `registry/index` (resolution metadata), and `git/db` (bare source download repositories). Follow explicit `CARGO_HOME`, resolving relative values against invocation cwd. Otherwise follow `$HOME/.cargo` only inside the workspace; fallback selection is `<cwd>/.cache/cargo`.
- Fallback selection is NOT a relocation: set `CARGO_HOME=.cache/cargo` for the dependency command; custom values must be consistent across restore/install/save. Run all three from the same directory. No native config is written or parsed.
- Key: existing content MD5 of the selected lockfile, 32 lowercase hex. Mapping insertion is immediately before Go; other tool entries retain relative order. Custom mounts still bypass preparation.
- Exclude the whole Cargo home, credentials/config at its root, bin, rustup toolchains, target, extracted registry/src and git/checkouts. Tests assert exact selected paths and unchanged project configuration. Archive inspection below verifies actual saved content for the registry fixture.

## Evidence
Local logs are under `.git/cache-intel/evidence/` (not product files). All CLI invocations below include `--tool cargo --worker-id w2 --branch feat/php-caching-support`.

| Check | Result | Evidence |
|-------|--------|----------|
| Baseline focused tests | PASS | `go test -count=1 ./internal/plugin/autodetect ./internal/plugin`: both packages passed before edits |
| Unit / scenario tests | PASS | CLI `create`; `test --local-test`; `test --compatibility`; root/nested/depth, env/default home, key changes, exclusion, idempotence, mixed Composer and preparation bypass cases |
| Focused package tests / vet | PASS | CLI `test --local-test`: autodetect and plugin passed; `go vet ./...` exit 0 |
| Cache tests | PASS | `go test -count=1 ./cache/...`: passed |
| Full repository tests | BLOCKED | `go test -count=1 ./...` exit 1; `full-test.log`: S3 localhost HTTPS requests fail `connect: permission denied`. No global suite pass claimed |
| Race | BLOCKED | `go test -race -count=1 ./...` exit 2: CGO disabled. Retry `CGO_ENABLED=1 go test -race -count=1 ./internal/plugin/autodetect ./internal/plugin` exit 1: gcc absent (`race-cgo.log`) |
| Build | PASS | `go build ./...` exit 0; CLI `build` exit 0 cross-compiled linux/amd64 and windows/amd64 and built host binary; `build.log` |
| CLI local lifecycle | PASS | CLI `test --lifecycle` exit 0; `cargo-lifecycle.log` shows cold/save/wipe/warm, 2/3 paths restored (optional git/db absent) |
| Real manager consumed restore | PASS | `python3 .git/cache-intel/evidence/manual-lifecycle.py` exit 0; after wiping `.cache` and `target`, restored `.crate` exists while registry/src is absent; `cargo fetch --locked --offline` and `cargo check --locked --offline` both exit 0 (`cargo-manual.log`) |
| Local invalidation | PASS | Same manual command: lockfile comment change changed `f11812e97cd532fb4658535fbf344710` to `cc4f15336d2ebc1023b389ae087d1e72`; restore requests new namespace and misses |
| Archive inspection | PASS | Same manual command runs `tar -tf` on both backend archives, asserts no credentials.toml, registry/src, git/checkouts, target or bin; archive listing contains crate archive, sparse index metadata and config.json |
| Legacy key / overrides characterization | PASS | CLI compatibility gate executes `TestScenario_Cargo_LegacyMixedKey` and key/override cases; existing package tests pass |
| Baseline/candidate cross-version archive interoperability | PENDING | Not independently exercised. No archive or backend implementation changes; local lifecycle uses candidate both ways, not baseline |
| Harness cold | PENDING | Not run in implement phase; no execution URL |
| Harness warm | PENDING | Not run in implement phase; no execution URL |
| Harness invalidation | PENDING | Not run in implement phase; no execution URL |

## Evidence provenance
- Documentation observed with `curl -fsSL https://doc.rust-lang.org/cargo/guide/cargo-home.html`: Cargo home defaults, CARGO_HOME override, internal-layout instability, and index/cache/git-db CI recommendations. These are current Cargo Book claims, not a claim that all versions were tested.
- `cargo --version`: 1.85.0 (d73d2caf9); `rustc --version`: 1.85.0 (4d91de4e4). Installed with `CARGO_HOME=/tmp/cache-intel-cargo RUSTUP_HOME=/tmp/cache-intel-rustup sh /tmp/cache-intel-rustup.sh -y --profile minimal --default-toolchain 1.85.0 --no-modify-path`. Local lifecycle uses direct toolchain binaries on PATH, Linux amd64.
- Public fixture inspected: `curl -fsSL https://raw.githubusercontent.com/sharkdp/hexyl/v0.15.0/Cargo.lock` and matching Cargo.toml both exit 0. Manifest declares Rust 1.74 minimum; lockfile version 3. Remote install remains untested.
- Local fixture is synthetic, committed under `test/fixtures/cargo-simple`, pinned to itoa 1.0.14 with committed checksummed lockfile. `cargo generate-lockfile`, `cargo fetch --locked`, offline fetch/check all passed. Cargo path internals beyond these observations remain version-sensitive.

## Command ledger
- PASS: CLI help first, then status, research, design, create, test --local-test, test --compatibility, test --lifecycle, build, finalize; all gates rerun after the final code change as applicable.
- PASS: direct focused baseline tests, `go test -count=1 ./cache/...`, `go build ./...`, `git diff --check`, manual lifecycle and archive inspection above.
- Corrected failure: lifecycle CLI reached successful warm restore but returned 1 from a RETURN trap referencing out-of-scope local variables (`cache_root: unbound variable`). Minimal CLI fix captures temporary paths when installing the cleanup trap; rerun exit 0. Included this supporting harness fix, not an engine behavior change.
- Corrected experiment: manual install with an empty environment could not reach the registry; retried with normal inherited routing and PLUGIN_DEBUG=false. No proxy settings changed; no environment dumps or credentials recorded.
- BLOCKED: full tests (sandbox localhost restriction), race (missing C toolchain). Owner: validation environment maintainer; permit local test-server connectivity and provide gcc/CGO, then rerun commands above.
- PENDING: remote validation and cross-version archive interoperability. Native macOS/Windows validation not performed; cross-compilation alone is not native support evidence.
- NOT_APPLICABLE: Docker commands, image publication, git push, PR creation, shared Harness resource changes; prohibited in this phase.

## Measurements
Latest manual lifecycle command (`cargo-manual.log`):
- Cold fetch: 0.1962s; warm offline fetch: 0.0304s.
- Cold cargo check: 0.0612s; warm offline cargo check: 0.0606s. Both start without target outputs; no compilation-output caching claimed.
- Backend tar archives: index 15,360 bytes + crate cache 13,824 bytes = 29,184 bytes, uncompressed. Do not label these as compressed sizes.
- Save process: 0.0101s; warm restore process: 0.0084s. Tiny fixture suggests ~0.147s net local fetch savings; no remote performance projection. Registry cache can grow; production measurements and pruning policy remain to be assessed.

## Compatibility and migration
`TestScenario_Cargo_LegacyMixedKey` proves the Composer-only content digest unchanged and Cargo+Composer concatenation in mapping order. Other legacy-only autodetect tests pass via CLI. Newly detected Cargo or mixed projects intentionally receive a new cold key. No existing tool mapping is reordered. Commit Cargo.lock before restore and keep it unchanged through save; manifest-only/generated-lock transitions are not supported. These constraints avoid an engine-wide restore-plan change.

## Limitations and open decisions
- CLI lifecycle logs report partial restore for registry-only projects because git/db is absent; expected optional content, not a complete 3/3 hit.
- CLI lifecycle also logs missing cache-metadata output path (`open : no such file or directory`). File restoration and offline consumption are independently asserted; no metadata persistence success is claimed. No unrelated engine fix attempted.
- No Git dependency offline-consumption proof, native Windows/macOS run, private registry or concurrent writer validation. Trusted writers and properly isolated backend required for dependency source caches.
- The selection logic cannot communicate environment variables from a plugin container to later build containers. The explicit install wiring below is mandatory.
- Shared mapping/docs/profile edits are included because the task explicitly assigns them; no unrelated ecosystem changes.

## Feedback incorporated
Scenario and unit tests cover explicit cwd-relative CARGO_HOME (also for nested projects), read-only configuration, deterministic keys, excluded paths and root/one-level discovery. Cargo home selection uses the actual cwd, not HARNESS_WORKSPACE, to match Cargo invocation semantics. The cleanup trap fix was prompted by the failed CLI lifecycle gate, which was rerun rather than skipped.

## Remote validation plan
Exact inputs for the remote-validate phase:
- Organization: `default`
- Project: `qajt`
- Branch: `feat/cargo-caching-w2`
- Fixture repo URL: `https://github.com/sharkdp/hexyl.git`
- Fixture ref (pinned release tag): `v0.15.0`
- Fixture subdirectory: `.`
- Install image: `rust:1.85.0-bookworm`
- Install command: `CARGO_HOME=.cache/cargo cargo fetch --locked`
- Warm-check path relative to fixture: `.cache/cargo/registry/cache`
- Docker connector: `rpdockerhub`
- Candidate image: PENDING orchestration build; use the exact supplied candidate, never latest.
- Pipeline: use only the identifier printed by CLI remote-setup (expected `ci_intel_cargo_w2`), read rendered YAML before MCP create/update/run.

Do not run this in implement phase. The remote plugin default selection is fixture/.cache/cargo, and the install command explicitly wires Cargo to that same path. Remote correctness, timings and archive sizes are PENDING until actual execution logs exist.

## Rollout and rollback
After orchestration builds the candidate from this commit, run the isolated cold/warm phase and record execution URLs and measured evidence before release. Normal reviewed release only. Roll back by reverting this tool's mapping/preparer changes or returning to the known source baseline above; an image rollback does not reuse newly introduced Cargo composite keys. No baseline image tag was supplied.
