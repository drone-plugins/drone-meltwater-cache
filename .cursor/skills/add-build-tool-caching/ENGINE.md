# drone-cache Autodetect Engine Reference

Repository-specific facts that constrain every design. Re-verify against the current checkout before relying on them; if code and this file disagree, the code wins and this file should be updated.

## Contents

- Detection pipeline
- Key composition
- Preparers and reusable helpers
- Plugin integration
- Local lifecycle recipe
- Building a candidate image
- Documentation and existing assets
- Known repository pitfalls

## Detection pipeline

Entry point: `detectDirectoriesToCache` in `internal/plugin/autodetect/auto_detect_util.go`. Each `buildToolInfo` entry in `buildToolInfoMapping` has:

| Field | Behavior |
|-------|----------|
| `globToDetect` | File pattern proving the tool is used |
| `tool` | Identifier logged and used for dedup |
| `preparer` | `RepoPreparer.PrepareRepo(dir) (string, error)` returns the primary cache path |
| `additionalCacheDirs` | `func(dir) ([]string, error)` returns extra paths; empty strings are ignored |
| `additionalHashGlob` | Extra file hashed into the key when the entry matches |
| `usePerProject` | Hash every match and prepare every matching directory (used by .NET) |
| `excludeIfExist` | Skip a match when any listed file exists in the same directory |

Behavior that designs must account for:

1. **Order is precedence.** Entries are evaluated in slice order. Once a `tool` identifier is detected, later entries with the same identifier are skipped. Put the stronger signal (lockfile) before the weaker one (manifest fallback).
2. **Discovery depth is root, then exactly one level.** The engine tries `<glob>` and then `**/<glob>`. Go's `filepath.Glob` does not support recursive `**`; it matches one directory segment. Projects two or more levels deep are not detected.
3. **One project per entry by default.** Unless `usePerProject` is set, only the shortest matching path is hashed and passed to `PrepareRepo`. `usePerProject` can prepare every match at the supported root/one-level depth, but enabling it changes key/path behavior and requires compatibility characterization.
4. **Path overrides disable preparation.** When `PLUGIN_MOUNT` is set, `skipPrepare` is true and no tools, paths, or hashes are produced.
5. **Detection runs in both restore and save steps**, usually in different containers. Both must reach the same key and paths from the workspace alone.

Anything outside these capabilities, such as recursive discovery beyond one level or key inputs the mapping cannot express, is an engine change. Treat it as a separate increment with its own compatibility characterization, because it can change keys and paths for existing tools.

## Key composition

- Each matched entry contributes the hex MD5 (32 characters) of the matched file. `additionalHashGlob` contributes another 32-character digest immediately before it.
- `additionalHashGlob` is resolved through the global root/one-level search, not relative to the directory selected by the primary detector. Characterize nested/hybrid repositories before using it; otherwise it can hash a different project's file.
- Contributions are concatenated in mapping order. The plugin then builds the final key from `PLUGIN_ACCOUNT_ID + "/" + <hashes>` through the metadata key template.
- `validatePlan` in `auto_key.go` requires the hash string to be a non-empty multiple of 32 lowercase hex characters. New contributions must keep this shape, or the package.json fallback plan breaks for mixed repositories.
- Consequences:
  - adding an entry that matches existing repositories changes their composite key, so their first run after rollout is cold;
  - reordering entries changes keys for mixed repositories;
  - hashing additional files for an existing tool changes that tool's keys.

## Preparers and reusable helpers

Reuse existing helpers before writing new ones:

| Helper | Location | Purpose |
|--------|----------|---------|
| `resolveRepoPath(dir, p)` | `prepare_python.go` | Clean absolute path; relative values resolve against the project directory |
| `workspaceRoot(dir)` | `prepare_node.go` | `HARNESS_WORKSPACE`, else the current directory |
| `pathWithin(path, root)` | `prepare_node.go` | Containment check for "default location already on the workspace" logic |
| `upsertTOMLString(path, section, key, value)` | `toml_config.go` | Idempotent, format-preserving TOML key update |
| `upsertEnv(path, key, value)` | `prepare_python.go` | Idempotent dotenv update |
| `upsertPipConf` / `iniHasKey` | `prepare_python.go` | INI-style update that respects an existing key |
| `readOptionalFile(path)` | `prepare_python.go` | Read content and preserve mode; absent file is not an error |

Two path strategies already exist. Choose one deliberately for each path:

- **Relocate**: write native project-local configuration so the tool uses `<project>/.cache/<tool>`. Examples: Poetry (`poetry.toml`), uv, pip (`pip.conf`), Pipenv (`.env`). Use this only when a later build step will discover that configuration without extra wiring, or document the wiring required. pip needs `PIP_CONFIG_FILE` and Pipenv needs `PIPENV_CACHE_DIR` in later steps.
- **Follow only**: never write configuration. Cache only a path that the environment or existing configuration already selects, or a default location that already lies within the workspace (`pathWithin`). Examples: npm cache and Composer cache. Use this when relocating would change developer behavior or the tool lacks reliable project-local configuration.

Conventions:

- The repository-local default is `<project>/.cache/<tool>`.
- An explicit environment variable wins over everything and is returned as-is through `resolveRepoPath`.
- Preparers must be idempotent and must not overwrite an existing user value for the same setting.

## Plugin integration

`internal/plugin/plugin.go` (the `cfg.AutoDetect` branch) defines the executable contract. Characterization tests are authoritative; comments are design clues only:

- The tool/key/path override matrix in the block comment identifies custom key/path cases, but tests must verify the surrounding control flow rather than assuming the comment is exact.
- Auto-detected paths use `cache.WithGracefulDetect(true)`, so absent auto paths are skipped. User-provided paths are not graceful.
- With no detected tool and no overrides, the step logs and exits successfully without caching.
- The package.json fallback plan (`WriteAutoDetectPlan` / `ReadAutoDetectPlan`) stores the restore-time key and path identities under `HARNESS_TMP_PATH`, scoped to the execution and stage. Save then reuses them, because install can create `package-lock.json` and change detection. If a new tool can create or change a detection file during install, extend this mechanism deliberately rather than inventing a parallel one.

## Local lifecycle recipe

Build the binary, then run restore, install, save, wipe, and restore against a disposable filesystem backend:

```bash
REPO_ROOT="$(pwd)"
PLUGIN_BIN="$REPO_ROOT/bin/drone-cache"
go build -o "$PLUGIN_BIN" .
CACHE_ROOT="$(mktemp -d)"
TEST_HOME="$(mktemp -d)"
FIXTURE_ROOT="<absolute-fixture-path>"
trap 'rm -rf "$CACHE_ROOT" "$TEST_HOME"' EXIT
common_env=(
  HOME="$TEST_HOME"
  PLUGIN_AUTO_CACHE=true
  PLUGIN_BACKEND=filesystem
  PLUGIN_FILESYSTEM_CACHE_ROOT="$CACHE_ROOT"
  PLUGIN_ACCOUNT_ID=test-account
  PLUGIN_OVERRIDE=true
  PLUGIN_LOG_LEVEL=debug
  PLUGIN_DEBUG=false
  DRONE_REPO_NAMESPACE=local
  DRONE_REPO_NAME=<fixture>
  DRONE_COMMIT_BRANCH=main
  DRONE_COMMIT_SHA=local-sha
)
# Clear the tool's own cache variables (e.g. UV_CACHE_DIR="") unless the scenario tests them.

cd "$FIXTURE_ROOT"
env -i "${common_env[@]}" PLUGIN_RESTORE=true  PLUGIN_REBUILD=false "$PLUGIN_BIN"  # cold: expect miss
<real install command>
env -i "${common_env[@]}" PLUGIN_RESTORE=false PLUGIN_REBUILD=true "$PLUGIN_BIN"   # save
find "$CACHE_ROOT" -type f -print -quit                                             # inspect stored archive
rm -rf <selected cache paths> <install outputs>
env -i "${common_env[@]}" PLUGIN_RESTORE=true  PLUGIN_REBUILD=false "$PLUGIN_BIN"  # warm: expect hit
<offline or instrumented install command proving consumption>
```

Use an explicit environment allowlist: `PLUGIN_DEBUG=true` logs every environment variable and must never be used with inherited host credentials. Run the real install in a disposable credential-free container or with its own minimal allowlist. Use a unique disposable backend and fixture copy for each tool/worker. To mirror Harness, run the install inside a container with the real toolchain image, and run the plugin binary separately against the same mounted workspace.

## Building a candidate image

The production images in `docker/Dockerfile.*` are `scratch` plus static busybox. They contain no package managers, so preparers must never shell out to the tool. Each Dockerfile copies a prebuilt binary:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/drone-cache .
docker build -f docker/Dockerfile.linux.amd64 -t <registry>/drone-cache:<branch>-<short-sha> .
```

Use `release/linux/arm64` with `Dockerfile.linux.arm64`, and `release/windows/amd64/drone-cache.exe` for Windows images. `.harness/harness.yaml` and `.drone.yml` are the authoritative release build definitions.

## Documentation and existing assets

- User docs: `README.md`, "Auto-Detection and Configuration" section. Record notable changes in `CHANGELOG.md`.
- Scenario test examples: `python_scenarios_test.go`, `composer_scenarios_test.go`.
- Smoke-script templates: `scripts/*-cache-smoke.sh`. These populate cache directories with dummy files, so they prove plugin plumbing only, not package-manager consumption.

## Known repository pitfalls

- The `Makefile` `test`, `test-integration`, and `test-e2e` recipes prefix `go test` with `-`, so make ignores test failures. Run `go test` directly for evidence.
- `make container` builds from a root `Dockerfile`, which does not exist. Use the `docker/Dockerfile.*` commands above.
- Do not hardcode developer-specific absolute paths (home directories, module caches) in scripts or tests; use `mktemp`, `t.TempDir()`, and environment defaults.
