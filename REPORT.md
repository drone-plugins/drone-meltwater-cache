# Cache Intelligence: cargo

## Summary
New tool support within the existing engine: Cargo lockfile autodetection and narrowly scoped dependency caches. Local CLI gates passed; the requested remote cold/warm gates passed for `rahulpx9/drone-cache:cargo-w2-21`. CLI `validate --env qa0 --tool cargo --worker-id w2 --branch feat/php-caching-support` returned `REMOTE_VALIDATED`; this label covers the CLI cold/warm criteria, not unexecuted remote invalidation or interoperability cases. Full-suite and race environment limitations are recorded below, not treated as passes. Evidence: `cache-intel.sh create`, `test --local-test`, `test --compatibility`, `test --lifecycle`, `build`, and `finalize`, all with `--tool cargo --worker-id w2 --branch feat/php-caching-support` (exit 0).

## Revisions
- Starting: `feat/php-caching-support` @ `2a04bac2a63abda51c0e0f38671c2da0eda70442` (`git rev-parse HEAD` before implementation).
- Ending: `feat/cargo-caching-w2`; implementation commit is the commit containing this report (`git log -1` after commit).
- Candidate image: `rahulpx9/drone-cache:cargo-w2-21`; pulled digest `sha256:7c003367892683a80d7ed483d508fc54de85d4bd45bbac52fe12c28381112f6d` (cold and warm restore step logs linked below). No local Docker command, git push or PR was run.
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
| Harness cold | PASS | [Run 1](https://app.harness.io/ng/account/h61p38AZSV6MzEkpWWBtew/all/orgs/default/projects/qajt/pipelines/ci_intel_cargo_w2/deployments/DybqDZTIRAaTtrxnJol8AA/pipeline): IgnoreFailed only for expected missing-cache restore; verify emits CACHE_STATE=cold / REMOTE_ASSERT=PASS; install and save complete, uploaded=2 missing=1 failed=0 |
| Harness warm | PASS | [Run 2](https://app.harness.io/ng/account/h61p38AZSV6MzEkpWWBtew/all/orgs/default/projects/qajt/pipelines/ci_intel_cargo_w2/deployments/Hefm3ycnTlmXZpEvlrcTxA/pipeline): Success; registry index/cache restored, CACHE_STATE=warm / REMOTE_ASSERT=PASS before install; no crate downloads in cargo fetch; save uploaded=2 missing=1 failed=0 |
| Harness invalidation | NOT RUN | Remote phase requests cold/warm only; local invalidation evidence above is not a remote test |

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
- PENDING: cross-version archive interoperability; remote invalidation was not requested in this phase and was not run. Native macOS/Windows validation not performed; cross-compilation alone is not native support evidence.
- NOT_APPLICABLE: local Docker commands, image publication, git push, PR creation and shared Harness resource changes; prohibited. Only assigned isolated pipeline `ci_intel_cargo_w2` was created and executed (MCP create/get/run results; execution links below).

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
- Candidate image used: `rahulpx9/drone-cache:cargo-w2-21` (restore logs in execution links below).
- Pipeline: use only the identifier printed by CLI remote-setup (expected `ci_intel_cargo_w2`), read rendered YAML before MCP create/update/run.

Do not run this in implement phase. The remote plugin default selection is fixture/.cache/cargo, and the install command explicitly wires Cargo to that same path. Remote cold/warm results, timings and archive sizes are recorded below from actual execution logs.

## Rollout and rollback
After orchestration builds the candidate from this commit, run the isolated cold/warm phase and record execution URLs and measured evidence before release. Normal reviewed release only. Roll back by reverting this tool's mapping/preparer changes or returning to the known source baseline above; an image rollback does not reuse newly introduced Cargo composite keys. No baseline image tag was supplied.


## Remote validation results (qa0, default/qajt)
Commands: CLI `remote-setup`, `remote-test --run cold --result PASS`, `remote-test --run warm --result PASS`, and `validate --env qa0`, each with `--tool cargo --worker-id w2 --branch feat/php-caching-support`; all exit 0. The remote-test commands included `--image rahulpx9/drone-cache:cargo-w2-21` and the respective execution URL. `validate` output: `PASS: autodetect tests`, `Terminal state: REMOTE_VALIDATED` (`.git/cache-intel/evidence/remote/validate.log`).

- [Cold execution](https://app.harness.io/ng/account/h61p38AZSV6MzEkpWWBtew/all/orgs/default/projects/qajt/pipelines/ci_intel_cargo_w2/deployments/DybqDZTIRAaTtrxnJol8AA/pipeline) — terminal IgnoreFailed, 91.734s total (MCP execution/diagnose). Restore reports three file-not-found misses; no credential/backend denial. `verify_restore`: `CACHE_STATE=cold`, `REMOTE_ASSERT=PASS`. `install`: `TIMING_install_seconds=1`, crate downloads present. `save_candidate`: requested=3 uploaded=2 missing=1 failed=0, took=627.729856ms. This meets the explicit cold criterion allowing IgnoreFailed with successful save.
- [Warm execution](https://app.harness.io/ng/account/h61p38AZSV6MzEkpWWBtew/all/orgs/default/projects/qajt/pipelines/ci_intel_cargo_w2/deployments/Hefm3ycnTlmXZpEvlrcTxA/pipeline) — terminal Success, 95.402s total (MCP diagnose). Restore: registry/index and registry/cache downloaded, partial 2/3 restoration in 315.826341ms. `verify_restore`: `CACHE_STATE=warm`, 16.4M registry/cache, `REMOTE_ASSERT=PASS` before install. `install`: `TIMING_install_seconds=0`, no Updating/Downloading lines. This supports restored dependency reuse with the exact documented CARGO_HOME wiring, but is not an offline remote test or functional build. `save_candidate`: requested=3 uploaded=2 missing=1 failed=0, took=527.810502ms.
- Archive bytes, both runs' save logs: registry/index **5,593,088**; registry/cache **17,149,440**; total **22,742,528**. Pipeline uses tar; these are exact upload sizes, not a claimed compression ratio. Same lockfile key in both runs: `28c048236f1d8b1893f68dee2e4ebff1` (restore/save logs).
- Whole-second install timers are coarse: warm `0` means under timer resolution, not zero work. Rough plugin restore+save overhead is 0.844s on warm run, close to the measured 1s cold fetch. No significant net pipeline speedup is demonstrated: VM/image/setup dominate, and warm total pipeline time was longer. Larger real workloads need separate value measurements.
- Both restore logs identify candidate digest above; Rust install logs identify `rust:1.85.0-bookworm` digest `sha256:0ff31c9ffa641a62e48d543fb00b4960955ea375f40776f40f585b89e654cc5e`. MCP pipeline get confirmed INLINE v0, Cloud Linux/Amd64, caching disabled, no shared templates, exact fixture tag/install wiring.
- Expected limitation: optional git/db absent for this registry-only fixture; warm restore is explicitly 2/3, not full hit. Warm restore also logs non-fatal `writeCacheMetadata ... open : no such file or directory`; file restore and consumption succeeded, metadata persistence is not validated. No plugin edits made in this phase.
- Log retrieval: MCP `harness_get execution_log` signed URLs followed by `fetch-step-log.sh '<url>' 200`. Initial fetch lacked unzip; reran successfully with a disposable Python zipfile-compatible unzip shim on PATH (no repository script changes). Sanitized decoded output retained under `.git/cache-intel/evidence/remote/{cold,warm}-<step>.log` for restore_candidate, verify_restore, install, save_candidate. Those paths are local evidence, not committed product files; execution links are durable report references.
- MCP warm run call timed out waiting for its response; execution list confirmed run 2 had started, so no duplicate was launched. Diagnosis polled that execution through Success. Both remote runs were recorded only after checking logs; no fixture retries or pipeline updates were needed.
- Still NOT RUN remotely: lockfile invalidation, offline functional build, native non-Linux platforms, Git dependencies, baseline/candidate archive interchange. Prior local evidence remains scoped as local. CLI terminal state is not a claim that these broader skill scenarios passed.

Remote scope of writes: MCP created only `ci_intel_cargo_w2`; no templates, connectors, secrets, settings or other pipelines modified. Publication was performed upstream by orchestration, not by this worker. Rollout remains a normal reviewed release after considering the documented limitations; this phase validates the supplied candidate, not a production rollout.
