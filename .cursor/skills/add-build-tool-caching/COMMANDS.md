# Command Reference

Use this file with `SKILL.md`. Every workflow command is a gate implemented by the repo CLI. A non-zero exit means the gate did not pass: fix the cause, or fill the requested document, and re-run the same command.

```bash
CLI=./.cursor/skills/add-build-tool-caching/scripts/cache-intel.sh
$CLI <command> --tool <name> --worker-id <id> [options]
```

Natural-language and slash-style requests (`/add-build-tool-caching research --tool uv`) map onto these commands. Never require exact syntax from the user.

## Order and gates

```text
research → design → create → test --all → build → finalize → harness-core      (implement phase)
remote-setup → remote-test cold → warm → invalidate [→ product-seed → product-warm]   (remote phases)
validate → report
```

| Command | Passes when | Writes |
|---------|-------------|--------|
| `research` | every Field is filled, `cache_env` is `none` or `KEY=relative-dir`, no TODO remains, and Sources has two or more URLs | `research.md` (scaffold on first run, exit 2) |
| `design` | research passed and every design section is filled | `design.md` (scaffold on first run, exit 2) |
| `create` | design passed, the tool is in `buildToolInfoMapping`, and at least one test whose name contains the tool passed | `evidence/create.log` |
| `test --local-test` | `go test` (autodetect, plugin, cache), `go vet ./...`, `go build ./...`, and gofmt on changed files | `evidence/test-local.log` |
| `test --compatibility` | at least one `*Scenario*<tool>*` test passed, plus the full autodetect suite (legacy keys and paths) | `evidence/test-compatibility.log` |
| `test --lifecycle` | in-repo fixtures from `tool-profiles.env` restore after a wipe (SKIPPED when there is no profile or the tool is missing) | `evidence/lifecycle-*.log` |
| `test --public` | the public fixture from research survives the full lifecycle below | `metrics.env`, `evidence/public-*` |
| `build` | linux/amd64, linux/arm64 and windows/amd64 binaries compile | `release/` |
| `finalize` | README.md and CHANGELOG.md mention the tool | |
| `harness-core` | NOT_APPLICABLE when `cache_env` is none or already injected; otherwise the env injection is committed in the harness-core checkout (BLOCKED when there is no checkout) | `out/harness-core.*` |
| `remote-setup` | research passed and `--image` is given | `remote/<pipeline>.yaml` |
| `remote-test` | `--result PASS` (records a Harness run; FAIL exits 1) | `remote/runs.tsv` |
| `validate` | no gate FAILED and none is missing | `out/summary.env` |
| `report` | always (regenerates from state) | `REPORT.md`, `out/PR_BODY.md`, `out/HARNESS_CORE_PR.md` |

Utilities: `help`, `status` (gate table and the next command), `waive --gate <g> --result BLOCKED|NOT_APPLICABLE|SKIPPED --reason "<owner and action>"`, `fixture-commit --from <dir> [--files a,b]`, and `all` (implement-phase gates, then validate and report).

## `research`

Follow `SKILL.md` triage and sections 1 and 2, starting from `TOOL-PATTERNS.md`. The first run writes `research.md` from [scripts/research.tmpl.md](scripts/research.tmpl.md) with the "Existing support" section prefilled (current mapping entries, preparers, and the variables harness-core already injects). Fill it from official documentation and from commands you ran against the real tool; then re-run `research`.

The Fields list is machine-read. Later gates take their inputs from it, so they need no extra flags:

- `cache_paths`, `excluded_paths`, `warm_check`: workspace-relative; the public lifecycle scans the saved archive for every excluded segment.
- `cache_env`: the variable that build steps need so the tool writes into the cached directory, as `KEY=workspace-relative-dir`, or `none`. A value here drives the harness-core change and the stage variable in the remote pipelines.
- `cache_env_side_effects`: harness-core will set `cache_env` for every step of every stage with caching on, not only the install step. List everything else that variable relocates (for `CARGO_HOME`: `cargo install` binaries in `bin/`, `config.toml`, `credentials.toml`) and how users keep it working (project-level `.cargo/config.toml`, `CARGO_REGISTRIES_*` env). It becomes the Compatibility section of the harness-core PR; reviewers decide on it.
- `fixture_repo`/`fixture_ref`: a public https repository pinned to a tag or commit with a committed lockfile. Prefer a real project with 50 or more locked dependencies so the cold/warm difference is measurable.
- `install_image`, `install_cmd`, `offline_cmd`: the install command must not wire the cache env itself (the CLI and the pipeline inject it). `offline_cmd` proves the tool consumes the restored data without network; use `none` only when the tool has no offline mode, and say so in Limitations.
- `harness_code_repo`: Harness Code repository holding a copy of the fixture, used by the product path.

Do not edit production code during `research`.

## `design`

Follow `SKILL.md` section 3 and `ENGINE.md`. The scaffold ([scripts/design.tmpl.md](scripts/design.tmpl.md)) asks for the summary, mapping entry and placement, the path table, the scenario matrix (each row names the test that covers it), compatibility and migration, and limitations. Summary, Compatibility and Limitations are copied verbatim into `REPORT.md` and the pull request description, so write them for reviewers.

## `create`

Follow `SKILL.md` sections 4 and 5. Implement the preparer and mapping entry with unit and scenario tests in the same change. Name tests after the tool (`Test_cargoPreparer_*`, `TestScenario_Cargo_*`): `create` and `test --compatibility` select tests by name and fail when none ran.

## `test`

```bash
$CLI test --tool <t> --worker-id <w> --all            # local-test, compatibility, lifecycle, public
$CLI test --tool <t> --worker-id <w> --public [--runner auto|host|docker]
```

`--public` clones the research fixture into a scratch workspace and runs the real tool against the plugin built from this checkout, using the filesystem backend and an isolated plugin environment (`env -i`):

1. cold restore must miss;
2. install (timed) must populate `warm_check`;
3. save; the archive listing must not contain any `excluded_paths` segment;
4. `git clean -ffdx` must remove the cache (proves it lives in the workspace);
5. warm restore must populate `warm_check`;
6. `offline_cmd` must succeed from the restored data alone;
7. warm install (timed);
8. changing `invalidate_file` must produce a different key, and the restore must miss.

The research `cache_env` is set for both the plugin and the tool, exactly as harness-core will inject it. `--runner auto` uses the tool on PATH, otherwise Docker with `install_image`; the agent container has no Docker, so install the tool in user space there (`SKILL.md` section 6). On macOS Docker bind mounts, the first tool invocation after a restore can be slow (file-system overhead); trust the remote timings for performance claims.

## `build`

Cross-compiles the release binaries and records the linux/amd64 checksum. The agent never builds or pushes images: the orchestration pipeline builds `docker/Dockerfile.linux.amd64` with `BuildAndPushDockerRegistry` after the implement phase, and passes the tag to the remote phase.

## `harness-core`

When research sets `cache_env`, Cache Intelligence must inject it into every step, like `GOCACHE`, `GRADLE_USER_HOME`, `PIP_CACHE_DIR` and `CP_CACHE_DIR`. The command runs [scripts/harness-core-env.py](scripts/harness-core-env.py) against the harness-core checkout (default `<git-common-dir>/cache-intel/harness-core`, prepared by the pipeline) and commits on `feature/[<JIRA>-]cache-intel-<tool>-env-<worker>`:

- `ContainerExecutionConstants`: `<TOOL>_CACHE_ENV_NAME`, `_CACHE_DIR` (`/harness/<dir>/`), `_CACHE_DIR_WIN`, `_CACHE_DIR_SUFFIX`, with `cache_env_reason` as the comment;
- `VmInitializeUtilsImpl` (`fullWorkDir + suffix`) and `K8InitializeTaskUtils` (`/harness/<dir>/`), Linux/macOS and Windows branches;
- assertions in `VmInitializeUtilsImplTest` and both `K8InitializeTaskUtilsTest` classes.

Pass `--jira <KEY>` when the pipeline has one; a missing Jira is not a blocker (the PR title can be edited later). Use `--no-windows` when the tool has no Windows support. Exit code 3 from the generator means harness-core already injects the variable; the gate is then NOT_APPLICABLE. The pipeline pushes the branch and opens the harness-core pull request; the agent never pushes.

## `remote-setup`

```bash
$CLI remote-setup --tool <t> --worker-id <w> --image <repo:tag> [--docker-connector rpdockerhub] \
  [--fixture-source github|harness_code] [--product --harness-code-repo <id> [--harness-code-branch main]]
```

Renders `ci_intel_<tool>_<worker>` from [scripts/remote-pipeline.tmpl.yaml](scripts/remote-pipeline.tmpl.yaml), all values coming from research:

- the fixture is cloned into `/harness` itself, and `cache_env` is a stage variable (`KEY=/harness/<dir>`), which is exactly what the harness-core change injects;
- a `scenario` input drives deterministic runs: `cold` skips restore (true cold baseline) and saves with `PLUGIN_OVERRIDE=true`; `warm` must restore before install, then runs `offline_cmd` and the install; `invalidate` changes `invalidate_file`, must miss, and never saves;
- the candidate image runs restore and save directly against the Harness cache backend (`HARNESS_CACHE_SERVICE_TOKEN`/`_ENDPOINT`, `HARNESS_ACCOUNT_ID`); stage `caching` stays disabled.

With `--product` it also renders `ci_intel_<tool>_<worker>_product` from [scripts/remote-product-pipeline.tmpl.yaml](scripts/remote-product-pipeline.tmpl.yaml): a Harness Code codebase, `caching.enabled: true`, and no plugin steps. It runs the real Cache Intelligence path, so it is meaningful only while the orchestrator has overridden the account-level cache image (`cacheTag`) to the candidate. Never change that override yourself.

To create the Harness Code fixture: copy only what the tool needs to resolve dependencies (manifest, lockfile, stub targets) into a scratch directory, run `$CLI fixture-commit --from <dir>`, create the repository with `harness_create resource_type=repository`, then commit `remote/fixture-commit.json` with `harness_create resource_type=commit`. Keep the commit small; its body passes through the MCP call.

Then, with the Harness MCP server for the environment: preview the YAML once, `harness_create resource_type=pipeline` (or `harness_update` when it exists), and re-read it to confirm the image and fixture.

## `remote-test`

Run the scenarios in order and wait for each to finish: `cold`, `warm`, `invalidate`. Inspect logs, not only statuses:

```bash
# harness_get resource_type=execution_log params {execution_id, stage_id: validate, step_id: <step>, return_download_url: true}
./.cursor/skills/add-build-tool-caching/scripts/fetch-step-log.sh '<download_url>' 200
```

| Scenario | Must see |
|----------|----------|
| cold | `verify_restore`: `CACHE_STATE=cold`, `REMOTE_ASSERT=PASS`; `install`: `TIMING_install_seconds`; `save_candidate`: upload of the cached paths |
| warm | `restore_candidate`: `build tools detected: <tool>` and downloads; `verify_restore`: `CACHE_STATE=warm`; `offline_check`: `OFFLINE_CHECK=PASS`; a smaller `TIMING_install_seconds` |
| invalidate | `invalidate_key` changed the hash; `verify_restore`: `CACHE_STATE=cold`, `REMOTE_ASSERT=PASS` |
| product-seed / product-warm | the Cache Intelligence restore step logs `build tools detected: <tool>` (the baseline image cannot detect a new tool, so this proves the override took effect); product-warm `verify_restore`: `CACHE_STATE=warm` |

Record each run, with metrics taken from the logs:

```bash
$CLI remote-test --tool <t> --worker-id <w> --run warm --execution-url <url> --result PASS \
  --metrics install_seconds=0.41,cache_kb=42500,restore_state=warm
```

A green execution alone is not evidence. If a run fails, diagnose (`harness_diagnose`, step logs), fix, rebuild only when code changed, and re-run the scenario. Do not delete shared caches; `cold` overwrites its own key.

## `validate`

Re-runs the full autodetect suite, then evaluates every gate from the ledger and prints `PASS`, `FAIL`, `BLOCK` or `MISS` per gate. Terminal state: `FAILED` when any gate failed, `INCOMPLETE` when a required gate is missing, `LOCAL_COMPLETE` without `--env`, `REMOTE_VALIDATED` when cold, warm and invalidate all passed, otherwise `REMOTE_BLOCKED`. It exits non-zero only for `FAILED` and `INCOMPLETE`.

## `report`

Generates, from state only (never hand-edit the output; fix the inputs and regenerate):

- `REPORT.md` at the repository root: at-a-glance table, summary, every gate with evidence, the local public-project lifecycle with timings, the Harness runs table with links, the harness-core change, research and design narrative, compatibility, limitations, rollout, and the collapsible command ledger;
- `out/PR_BODY.md`: the GitHub pull request description;
- `out/HARNESS_CORE_PR.md`: the harness-core pull request description (only when the harness-core gate committed a change).

Commit `REPORT.md` with the change. Never convert an unverified path into a supported-tool claim.

## `status` and `help`

`status` prints the gate table and the next command; it changes nothing. `help` prints usage, the state layout, and how images and logs are handled.

## Harness MCP tool map

- discovery: `harness_describe`, `harness_schema`, `harness_search`;
- reads: `harness_list`, `harness_get`, `harness_status`;
- writes: `harness_create`, `harness_update` (only the worker's own `ci_intel_<tool>_<worker>*` pipelines and `cache_intel_fixture_<tool>_<worker>` repositories);
- execution: `harness_execute`;
- failure analysis: `harness_diagnose`.

Never use `harness_delete`, and never modify templates, connectors, secrets, account settings or the CI execution config. Surface governance or policy failures as blockers.
