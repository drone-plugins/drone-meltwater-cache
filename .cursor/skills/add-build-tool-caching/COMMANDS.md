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
remote-setup → remote-test cold → warm → invalidate                             (remote-validate phase)
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

Utilities: `help`, `status` (gate table and the next command), `waive --gate <g> --result BLOCKED|NOT_APPLICABLE|SKIPPED --reason "<owner and action>"`, `fixture-commit --from <dir> [--files a,b] [--replace <paths-file>]` (writes `remote/fixture-commit.json` for `main` and `remote/fixture-commit-invalidate.json` for the `invalidate` branch; `--replace` adds the commits that swap out another project, see `remote-setup`), and `all` (implement-phase gates, then validate and report).

## `research`

Follow `SKILL.md` triage and sections 1 and 2, starting from `TOOL-PATTERNS.md`. The first run writes `research.md` from [scripts/research.tmpl.md](scripts/research.tmpl.md) with the "Existing support" section prefilled (current mapping entries, preparers, and the variables harness-core already injects). Fill it from official documentation and from commands you ran against the real tool; then re-run `research`.

The Fields list is machine-read. Later gates take their inputs from it, so they need no extra flags:

- `cache_paths`, `excluded_paths`, `warm_check`: workspace-relative; the public lifecycle scans the saved archive for every excluded segment.
- `cache_env`: the variable that build steps need so the tool writes into the cached directory, as `KEY=workspace-relative-dir`, or `none`. A value here drives the harness-core change and the stage variable in the remote pipelines.
- `cache_env_side_effects`: harness-core will set `cache_env` for every step of every stage with caching on, not only the install step. List everything else that variable relocates (for `CARGO_HOME`: `cargo install` binaries in `bin/`, `config.toml`, `credentials.toml`) and how users keep it working (project-level `.cargo/config.toml`, `CARGO_REGISTRIES_*` env). It becomes the Compatibility section of the harness-core PR; reviewers decide on it.
- `fixture_repo`/`fixture_ref`: a public https repository pinned to a tag or commit with a committed lockfile. Prefer a real project with 50 or more locked dependencies so the cold/warm difference is measurable.
- `install_image`, `install_cmd`, `offline_cmd`: the install command must not wire the cache env itself (the CLI and the pipeline inject it). `offline_cmd` proves the tool consumes the restored data without network; use `none` only when the tool has no offline mode, and say so in Limitations.
- `harness_code_repo`: Harness Code repository holding a copy of the fixture, cloned by the remote pipeline; `none` means `cache_intel_fixture_<tool>_<worker>`.

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

With an `offline_cmd`, step 6 is the first to read the restored cache and the install in step 7 has nothing left to fetch, so the reported warm time is step 6's. The fixture this command ran on is recorded (`LOCAL_FIXTURE_REPO`, `LOCAL_FIXTURE_REF`), and `remote-setup` refuses a research fixture that differs from it.

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
  [--k8s-connector deleciplay] [--k8s-namespace harness-delegate-ng] [--harness-code-repo <id>] [--harness-code-branch main]
```

Renders `ci_intel_<tool>_<worker>` from [scripts/remote-pipeline.tmpl.yaml](scripts/remote-pipeline.tmpl.yaml), all values coming from research. It validates the real product path:

- Kubernetes stages (`KubernetesDirect`, default `deleciplay` / `harness-delegate-ng`) with `caching.enabled: true` and no plugin steps: restore and save are the steps Cache Intelligence injects (`restore-cache-harness`, `save-cache-harness`), backed by the project's self-hosted cache connector and bucket;
- those steps run the candidate only because the orchestrator pins it in the account-level K8s cache tags (`cacheGCSTag`, `cacheS3Tag`, `cacheTag`, `infra=K8`) for the remote-validate phase. Never change that setting yourself;
- the codebase is the Harness Code fixture repository, because the injected restore runs right after the clone and before any user step;
- `cache_env` is a stage variable (`KEY=/harness/<dir>`), which is exactly what the harness-core change injects;
- a `scenario` input runs only the stage of the same name, each with a fixed cache policy: `cold` uses `push` (no restore, true cold baseline, then save with override); `warm` uses `pull-push`, must restore before install, then runs `offline_cmd` and the install; `invalidate` clones the fixture's `invalidate` branch (changed `invalidate_file`), uses `pull`, must miss, and never saves.

The cache key is the hash of the detected files only, so every candidate for the same fixture shares it. That is why `cold` must not restore, and why `invalidate` never saves.

To create the Harness Code fixture: copy only what the tool needs to resolve dependencies (manifest, lockfile, stub targets) into a scratch directory, keeping `fixture_subdir`, and run `$CLI fixture-commit --from <dir>`. Create the repository with `harness_create resource_type=repository` (private, default branch `main`), commit `remote/fixture-commit.json`, then `remote/fixture-commit-invalidate.json` (it creates branch `invalidate` with `invalidate_file` changed by one trailing newline; the blob `sha` it carries is the one of the `main` commit). Keep the commits small; their bodies pass through the MCP call.

The repository outlives the run, so a later run of the same worker can find it holding another project. `fixture-commit` prints `FIXTURE_MATCH`, the blob sha `invalidate_file` has on each branch when the branch holds this fixture; compare it with the `sha` of `harness_get resource_type=file_content` for that path and branch. When both match, commit nothing. Otherwise never switch research to the project found there: the local lifecycle ran on the research fixture, and `remote-setup` fails when the two differ. Save the paths `harness_list resource_type=file_content` returns for `main` and re-run `fixture-commit --replace <file>`. Harness Code `UPDATE` needs each file's current blob sha, so the commits it lists in `FIXTURE_COMMITS` empty each branch with `DELETE` actions and then recreate the fixture with `CREATE`, `invalidate` included. Commit them in that order; if `invalidate` does not exist yet, commit `remote/fixture-commit-invalidate.json` in place of its two commits. For `repository` and `commit` calls pass `org_id` and `project_id` (and `params.repo_id` for commits) but omit `resource_scope`: the MCP server rejects `resource_scope=project` for these types ("Supported scopes: account"), while `org_id`/`project_id` alone address the project repository.

Then, with the Harness MCP server for the environment: preview the YAML once, `harness_create resource_type=pipeline` (or `harness_update` when it exists), and re-read it to confirm the infrastructure, the fixture repository and the stage variable.

## `remote-test`

Run the scenarios in order and wait for each to finish: `cold`, `warm`, `invalidate`. Each run executes only the stage named after its scenario, so that is the `stage_id` for its logs. Inspect logs, not only statuses:

```bash
# harness_get resource_type=execution_log params {execution_id, stage_id: <scenario>, step_id: <step>, return_download_url: true}
./.cursor/skills/add-build-tool-caching/scripts/fetch-step-log.sh '<download_url>' 200
```

| Scenario | Must see |
|----------|----------|
| cold | no `restore-cache-harness` step (policy `push`); `verify_restore`: `CACHE_STATE=cold`, `REMOTE_ASSERT=PASS`; `install`: `TIMING_install_seconds`; `save-cache-harness`: `build tools detected: <tool>` and the upload of the cached paths |
| warm | `restore-cache-harness`: `build tools detected: <tool>` and the download; `verify_restore`: `CACHE_STATE=warm`; `offline_check`: `OFFLINE_CHECK=PASS` and a `TIMING_offline_seconds` below the cold `TIMING_install_seconds` |
| invalidate | `verify_restore`: `FIXTURE=...@invalidate`, a `KEY_FILE_MD5` different from the cold run's, `CACHE_STATE=cold`, `REMOTE_ASSERT=PASS`; `restore-cache-harness`: `build tools detected: <tool>` and a not-found for every path under the new key; no `save-cache-harness` step (policy `pull`) |

When every path misses, the plugin exits non-zero (`restore failed`, then `silent fails disabled`), with the released image too: `failIfKeyNotFound` is only consulted when no paths are given, and Cache Intelligence always passes them. The injected restore carries an Ignore failure strategy, so that step and the invalidate run end `IgnoreFailed`. That is the expected miss: record PASS when the markers above hold. Invalidate fails only when a path was restored, `verify_restore` fails, or a save step ran.

`build tools detected: <tool>` is the proof that the candidate ran: the baseline image cannot detect a new tool. When it is missing, the account-level image pin did not take effect (or the project has no self-hosted cache connector, in which case Cache Intelligence uses the service image); record BLOCKED with that evidence, not FAIL.

Record each run, with metrics taken from the logs. For warm, `install_seconds` is `TIMING_offline_seconds`: `offline_check` is the first step that reads the restored cache, so the install after it has nothing left to fetch. Use the install's `TIMING_install_seconds` only when `offline_check` printed `OFFLINE_CHECK=skipped`.

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
- writes: `harness_create`, `harness_update` (only the worker's own `ci_intel_<tool>_<worker>` pipeline and `cache_intel_fixture_<tool>_<worker>` repository, including its commits);
- execution: `harness_execute`;
- failure analysis: `harness_diagnose`.

Never use `harness_delete`, and never modify templates, connectors, secrets, account settings or the CI execution config. Surface governance or policy failures as blockers.
