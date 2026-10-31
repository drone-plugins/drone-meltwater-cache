# Research: {{TOOL}}

<!--
Fill every field and section, replacing each TODO, then re-run `cache-intel.sh research`.
The CLI reads the Fields list for later gates (public lifecycle, remote pipelines, harness-core),
so later commands need no extra flags. Write `none` for a field that does not apply.

Field reference:
- tool_id: identifier used in buildToolInfoMapping (e.g. cargo, python-uv)
- tool_versions: versions you verified (e.g. cargo 1.85.0)
- detect_files: detection globs, strongest first (e.g. Cargo.lock)
- key_inputs: files hashed into the key
- cache_paths: workspace-relative paths the plugin selects (comma-separated)
- excluded_paths: path segments that must never be archived (comma-separated, e.g. target,registry/src)
- strategy: follow-only | relocate
- cache_env: KEY=workspace-relative-dir that build steps need so the tool writes into the cached dir, or none
- cache_env_reason: one sentence on why Harness must inject cache_env (becomes the harness-core code comment), or none
- cache_env_side_effects: what else the variable moves for every step of every stage with caching on (config files, credentials, installed binaries) and how users keep them working, or none
- fixture_repo / fixture_ref / fixture_subdir: small public project with a committed lockfile, pinned tag or commit
- install_image: pinned image that has the tool (used remotely and by `test --public --runner docker`)
- install_cmd: dependency install command WITHOUT cache env wiring (cache_env is injected separately)
- offline_cmd: command that must succeed from the warm cache alone (e.g. cargo fetch --locked --offline), or none
- warm_check: workspace-relative directory that must be non-empty after a warm restore
- invalidate_file: workspace-relative file whose change must change the key (usually the lockfile)
- unset_env: tool env vars to clear during isolated local runs (comma-separated), or none
- harness_code_repo: Harness Code repo identifier holding a copy of the fixture (product path), or none
-->


## Fields
- tool_id: {{TOOL_SLUG}}
- tool_versions: TODO
- detect_files: TODO
- key_inputs: TODO
- cache_paths: TODO
- excluded_paths: TODO
- strategy: TODO
- cache_env: TODO
- cache_env_reason: TODO
- cache_env_side_effects: TODO
- fixture_repo: TODO
- fixture_ref: TODO
- fixture_subdir: .
- install_image: TODO
- install_cmd: TODO
- offline_cmd: none
- warm_check: TODO
- invalidate_file: TODO
- unset_env: none
- harness_code_repo: none

## Existing support
{{EXISTING_SUPPORT}}

## How {{TOOL}} resolves its cache
TODO: default location per OS, environment variables, native config files and their precedence, and how relative paths resolve.

## What to cache and what not
TODO: classify every candidate path (download cache / installed tree / build output / toolchain / metadata), with reuse, portability, credential risk and expected size. Give a reason for every exclusion.

## Key and invalidation
TODO: which inputs fully determine the cached content, and what must produce a new key.

## CI integration
TODO: Harness runs restore, build and save in separate containers that share only the workspace. Explain how the build step ends up writing into the cached directory, and whether harness-core must inject cache_env (see "Existing support" for what it already injects).

## Fixture choice
TODO: why this public project (size, committed lockfile, deterministic install, license).

## Sources
- TODO: official documentation URL
- TODO: second authoritative URL (tool source, changelog, or reference docs)
