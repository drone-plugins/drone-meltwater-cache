---
name: add-build-tool-caching
description: Research, design, create, test, diagnose, and validate Cache Intelligence support through a command-based workflow for any language, package manager, or build tool. Use when adding or changing detection, cache paths, keys, repository preparation, restore/save behavior, plugin images, or local and Harness environment validation in drone-cache.
---

# Add Build Tool Caching

## Purpose

Use this procedure to add cache support for a language or build tool through small, evidence-based iterations. Do not treat the first implementation as final. Confirm requirements, test assumptions locally, validate the exact image in a real environment, collect feedback from logs and timings, and revise until the design is safe and repeatable.

The outcome is not merely a green pipeline. The implementation must:

- detect the intended projects without false positives;
- select cache content that is reusable, portable, and worth storing;
- generate stable keys that invalidate when dependencies change;
- preserve existing user configuration;
- tolerate expected cold-cache and missing-path conditions;
- work in local tests and the target CI environment;
- document limitations and operational evidence.

## Command interface

Treat the first argument as a workflow command:

```text
/add-build-tool-caching <command> [arguments]
```

Natural-language equivalents are valid. Read [COMMANDS.md](COMMANDS.md) before executing a command.

Available commands:

```text
help          Show command usage and examples
status        Report current evidence, blockers, and next command
research      Gather requirements and verify tool behavior
design        Define detection, paths, keys, compatibility, and tests
create        Implement one complete code-and-test increment
test          Run local tests and restore/save/restore validation
build         Build and identify the candidate binary/image
remote-setup  Discover or prepare Harness validation resources
remote-test   Run cold/warm/invalidation tests through Harness MCP
diagnose      Investigate local or remote failures
validate      Evaluate acceptance and compatibility gates
report        Produce evidence and rollout documentation
all           Execute the complete workflow in dependency order
```

Examples:

```text
/add-build-tool-caching research --tool cargo
/add-build-tool-caching create --tool uv
/add-build-tool-caching test --tool uv --local
/add-build-tool-caching remote-test --env qa0 --pipeline <id>
/add-build-tool-caching all --tool pnpm --env qa0
```

Select the narrowest matching command. Do not turn `research`, `test`, or `status` into implementation or remote mutation. For `all`, update command state and apply feedback after every phase.

For remote work, use the Harness MCP server associated with the requested environment. Establish account/org/project scope, verify dependencies, preview writes, use MCP approval for mutations, monitor executions to completion, and diagnose failures. Never substitute another scope or environment merely because a lookup fails.

## Operating principles

1. **Evidence before design**: inspect official tool behavior and real projects before choosing paths.
2. **Cache downloads, not fragile state**: prefer package/artifact caches over virtual environments, build outputs, credentials, locks, sockets, or machine-specific files.
3. **Respect user configuration**: environment variables and existing native configuration normally take precedence over generated defaults.
4. **Prefer repository-local defaults**: CI steps may only share the workspace. Use an external path only when it is explicitly configured and available across steps.
5. **Detect precisely**: use lockfiles when available. Define precedence and coexistence rules for hybrid repositories.
6. **Keep restore misses non-fatal**: a cold cache is normal. Distinguish a missing object from storage, permission, corruption, or extraction failures.
7. **Prove warm behavior**: a successful save is insufficient; run again and prove that the expected content restored.
8. **Pin test images**: environment validation must use the exact candidate image or immutable digest.
9. **Record feedback**: every failed assumption becomes a test, design update, or documented limitation.
10. **Do not broaden scope silently**: separate support for different managers unless they intentionally share behavior.
11. **Preserve executable contracts**: characterize current behavior before changing it; comments and intended behavior are not substitutes for a baseline.
12. **Prove build-step integration**: a selected path is useful only if restore, build, and save see the same persisted location.
13. **Keep credentials out**: never cache broad home/config roots that may contain tokens, registry authentication, or signing material.
14. **Prefer additive increments**: isolate each ecosystem or compatibility correction so it can be reviewed, validated, and rolled back independently.

## Required inputs

Before editing code, determine:

- target language and build tool;
- supported tool versions and operating systems;
- manifest and lockfile names;
- native cache configuration mechanisms and precedence;
- expected cache path with and without user configuration;
- dependency install command;
- target validation environment, pipeline type, connector, and image;
- compatibility requirements for existing tools and pipelines;
- success criteria, including correctness and expected performance evidence.

If any item materially changes the design and cannot be inferred, ask the user. Otherwise, state the assumptions and continue.

## Scope and safety
Before implementation:

- read repository instructions and preserve unrelated working-tree changes;
- record the starting SHA, branch, status, required toolchain, and direct baseline test results;
- compare the current checkout with the issue's assumed baseline so merged work is not reimplemented;
- do not push, merge, publish, release, deploy, or modify shared templates unless requested;
- use disposable fixtures, backends, keys, and cache roots—never a developer's real package cache or credentials;
- keep blocked ecosystem work separate and continue independent validated work.

## Procedure
### 1. Establish requirements

Inspect the issue, specification, existing implementation, and related build-tool support. Convert them into explicit acceptance criteria.

At minimum, answer:

- What file proves this tool is in use?
- Can the file appear at the repository root and in nested projects?
- Which files must affect the cache key?
- Which other managers may coexist in the same repository?
- Is one tool preferred over another, or should both caches be used?
- What configuration must remain untouched?
- What should happen when the cache path does not exist?
- What proves a cache hit is useful?
- Will adding this detector change a composite key for existing mixed projects?
- Can lockfiles be absent during restore but generated during install?

Produce a short requirements record:

```text
Tool:
Detection files:
Key inputs:
Default cache path:
Configuration precedence:
Coexistence/exclusion rules:
Unsupported content:
Validation environment:
Acceptance criteria:
Open questions:
```

**Gate:** Do not implement while detection, path ownership, or configuration precedence is ambiguous.

### 2. Research the build tool

Use authoritative documentation and direct commands from the tool where possible. Verify:

- the native command that prints its cache directory;
- relevant environment variables;
- native configuration files and precedence;
- whether relative paths resolve from the workspace, project, or home directory;
- whether configuration is read once at process startup;
- whether cache content is portable between images, architectures, and tool versions;
- whether concurrent reads/writes are supported;
- whether the cache may contain credentials or private source material.
- which OS, architecture, runtime, and package-manager versions are actually supported;
- whether the production plugin image can run the tool—do not assume it can; validation CLIs belong in disposable test/build images;
- what Harness shares between restore, build, and save containers, including workspace mounts and environment injection boundaries.

Inspect at least one representative open-source project. Prefer fixtures that are small, have committed lockfiles, and install deterministically.

Before production changes, add compatibility characterization for affected existing tools. Capture current keys, path order, configuration side effects, override behavior, and failures. Treat known limitations as baseline facts unless the requested increment explicitly changes them.

Classify every candidate path:

```text
Path:
Purpose:
Reusable across runs:
Portable across images:
Safe to archive:
Expected size:
Created before save:
Decision: cache / exclude / investigate
```

Do not cache an environment or output merely because it improves one warm run. For example, Python `.venv` and `venv` directories embed interpreter paths and should be rebuilt from the package-download cache.

**Gate:** Select only paths with a documented reason and known invalidation behavior.

### 3. Design detection, paths, and keys

Define the design before coding:

1. **Detection**
   - Prefer lockfiles over broad manifests.
   - Specify root and nested-project behavior.
   - Avoid duplicate detection when multiple patterns represent one manager.
   - For new ecosystems, use bounded deterministic discovery; exclude `.git`, dependency caches, virtual environments, generated output, and vendored trees.
   - Do not follow symlink cycles or silently change legacy monorepo discovery semantics.

2. **Tool identity**
   - Give independent managers independent identifiers, such as `python-uv` and `python-poetry`.
   - Share an identifier only when preparation, paths, and invalidation are intentionally identical.

3. **Coexistence**
   - Use exclusions only when two signals describe the same project and one must win.
   - Permit multiple tools when the repository genuinely uses multiple managers.

4. **Cache path**
   - Honor explicit environment configuration first.
   - Otherwise configure a repository-local path through the tool's native mechanism.
   - Merge with existing configuration; never replace unrelated settings.
   - Return the path the tool will actually use.

5. **Cache key**
   - Include lockfile content and other dependency inputs required for correctness.
   - Ensure dependency changes invalidate the key.
   - Avoid unstable machine-specific values.
   - Encode file boundaries and normalized project-relative identities so concatenated content cannot collide.
   - Do not include absolute checkout paths, secrets, or cache-directory existence.
   - Preserve legacy key order/algorithm unless an explicit migration is designed.

6. **Missing paths**
   - Auto-detected paths may be absent before the first install.
   - Save should skip an expected absent path with a clear summary.
   - Do not hide permission, I/O, archive, or backend failures as missing paths.

Write a small scenario matrix before implementation:

```text
Scenario | Detected tools | Cached paths | Key inputs | Expected result
single tool
nested project
manifest without lockfile
two lockfiles
tool plus generic manifest
user-defined cache path
existing native config
missing cache directory
changed lockfile
lockfile generated after restore
mixed legacy and new tools
platform/runtime boundary
```

**Feedback checkpoint:** Present meaningful precedence or portability trade-offs to the user before locking the design.

### 4. Implement the smallest complete slice
Follow existing repository patterns rather than creating a parallel framework.

Typical implementation areas:

- add manifest/lockfile mapping in `internal/plugin/autodetect/auto_detect_util.go`;
- add or extend a `RepoPreparer` in `internal/plugin/autodetect/prepare_<tool>.go`;
- merge native configuration without overwriting user values;
- resolve relative paths to clean absolute workspace paths;
- deduplicate paths and tool identifiers;
- add special handling only when the generic detector cannot express the requirement.

Keep preparation idempotent: running detection twice must not duplicate configuration or change the result.

Do not combine unrelated managers in one preparer merely because they share a language.

The plugin may run in a minimal image without the package manager. Infer paths from documented configuration; use the real CLI only in tests. If repository mutation is necessary, make it minimal, format-aware, idempotent, and atomic. Preserve comments, unrelated settings, authentication, permissions, and newline conventions. Fail clearly on malformed or read-only files instead of silently changing dependency behavior.

Do not assume an environment variable set inside the cache plugin reaches a later build container, or that one exported in a build script was visible to an earlier restore step.

### 5. Add tests before expanding the implementation

Add tests at three levels.

#### Unit tests

Cover:

- each detection file;
- expected tool identifier;
- default and environment-overridden paths;
- native configuration merge behavior;
- idempotency;
- nested projects;
- absent and malformed optional configuration;
- path normalization.

#### Scenario tests

Cover the design matrix, especially:

- coexistence and exclusions;
- duplicate manifest patterns;
- lockfile changes affecting the key;
- existing user configuration remaining unchanged;
- unsafe paths not being returned.

Use isolated environment variables and temporary directories. Tests must not depend on the developer's home-directory configuration.

#### Regression tests

Run focused package tests first, then direct repository gates with propagated exit codes:

```bash
go test -count=1 ./internal/plugin/autodetect ./internal/plugin
go test -count=1 ./cache/...
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

Run formatter/linter and build-tagged integration suites required by the repository. Check service readiness and which tests actually ran. Do not use a successful wrapper command as evidence when its recipes can ignore failures. If a suite requires external credentials, separate environment failures from code failures and report passed, failed, and skipped commands precisely.

Add compatibility tests where affected:

- baseline and candidate produce identical legacy-only keys, path order, overrides, and side effects;
- baseline-save restores with candidate, and candidate-save restores with baseline, using real archives;
- mixed old/new projects have an explicit key-transition decision;
- custom key/path, autodetect, detected/undetected, and preparation-bypass combinations retain executable behavior;
- restore and save remain consistent when lockfiles appear or change during install;
- repeated detection is deterministic and unrelated files do not change identity;
- missing optional paths stay graceful while permission, archive, backend, and I/O errors remain failures.

**Gate:** Do not build an image until focused tests pass and the scenario matrix is represented.

### 6. Validate locally with restore/save/restore

Exercise the actual cache lifecycle, not only autodetection:

1. Start with an empty backend namespace or unique key.
2. Run restore and confirm the miss is non-fatal.
3. Run the real dependency install command.
4. Confirm the selected path exists and contains expected artifacts.
5. Run save, inspect the archive, and verify it contains intended reusable data—no credentials, environments, or generated output.
6. Remove the local cache content.
7. Start a fresh checkout/execution with only the remote cache retained, then restore.
8. Confirm files were restored.
9. Run dependency install plus a functional import/build/test.
10. Change the lockfile and confirm key invalidation.

Test multiple paths independently. A partial restore can be valid when optional paths are absent, but investigate repeated missing candidates. Remove paths that are predictably absent or unsafe rather than accepting permanent noise.

Prove the package manager consumes restored data. Prefer supported offline mode, download instrumentation, or a controlled registry; a cache-hit log or restored dummy file is insufficient. Check tool semantics first: for example, a package manager's HTTP cache may accelerate requests without supporting fully offline installation.

For generated lockfiles, begin cold and warm executions from equivalent clean source state. Verify that save does not discover unrelated new paths under a restore-time key; reuse an execution-scoped plan only when necessary and preserve its validation and isolation.

Capture:

- detected tool identifiers;
- generated key;
- local and remote paths;
- cold and warm status;
- archive size;
- restore/save result;
- install duration where meaningful.
- tool, runtime, image, OS, and architecture versions;
- restore/save overhead and transferred bytes.

### 7. Build and pin a candidate image
Build and publish the candidate plugin image using the repository's established process.

Requirements:

- use a unique immutable tag tied to the branch or commit;
- record the image digest when available;
- do not test through `latest`;
- verify the pipeline references the candidate image or template version;
- record whether the pipeline is v0, v1, or another execution path;
- confirm no account-level template silently substitutes an older image.
- build supported release targets and smoke-test the candidate container without publishing solely for testing.

Keep baseline and candidate pipelines separate when comparing old and new behavior.

**Gate:** Before running, show the environment, pipeline, repository, branch, plugin image, and expected cache path.

### 8. Test in the target environment

Use a real representative project for every supported manager or mode.

For each case, run:

1. **Cold run**
   - use a unique cache key or clear namespace;
   - expect restore miss;
   - install dependencies;
   - expect selected paths to upload.

2. **Warm run**
   - use identical inputs;
   - expect restore hit for the selected paths;
   - confirm the dependency command consumes restored content;
   - expect save to succeed without unexpected missing paths.

3. **Invalidation run**
   - change a key input;
   - expect a different key and cold behavior.

4. **Compatibility cases**
   - existing user config;
   - user-defined cache environment variable;
   - nested or hybrid project if supported;
   - missing path;
   - different build image if portability is claimed.
   - restore/build/save in separate containers sharing only production-supported paths.

Do not infer a warm cache solely from a successful pipeline or optimization label. Inspect restore logs, cache-state markers, file counts, or tool-specific evidence. Timing is supporting evidence because bootstrap, network, and scheduling noise can dominate short installs.

Do not provide hidden mounts or environment wiring and then claim zero-configuration support. If companion runner/template changes are required, identify them concretely and keep them outside the plugin change unless requested. Cross-compilation and simulated paths do not prove native macOS or Windows support.

### 9. Run the feedback loop
After each local or environment run:

1. Compare actual detection, paths, keys, and outcomes with the scenario matrix.
2. Classify differences:
   - implementation defect;
   - incorrect requirement;
   - environment/template mismatch;
   - test-fixture issue;
   - expected cold/missing behavior;
   - performance noise;
   - unsupported case.
3. Update the design record.
4. Add or update a regression test before changing code.
5. Make the smallest correction.
6. Rerun focused tests, local lifecycle tests, then affected environment cases.

Ask for user feedback when:

- two valid cache-path strategies have different portability or performance;
- coexistence rules may change existing behavior;
- environment evidence contradicts documented tool behavior;
- rollout requires changing shared templates or default images;
- a limitation must be accepted rather than fixed.

Repeat until no unexplained differences remain.

### 10. Finalize

Update user-facing documentation with:

- supported tool and detection files;
- configuration precedence;
- default and overridden cache paths;
- coexistence behavior;
- key inputs;
- limitations and deliberately excluded paths;
- required environment variables for later build steps;
- cold/warm validation evidence.
- supported OS/runtime/tool versions and configuration side effects;
- migration effects, including expected cold starts when keys or paths change.

Use the `report` format in [COMMANDS.md](COMMANDS.md). Include starting/ending revisions, changed behavior, commands passed/failed/skipped, local and Harness evidence, measured reuse and overhead, compatibility and migration effects, feedback incorporated, unresolved limitations, and staged rollout/rollback guidance.

## Failure and rollback rules

- Stop if the candidate image is not actually used by the target pipeline.
- Stop on credential, permission, or backend errors; do not classify them as cache misses.
- Do not weaken validation merely to make a run green.
- Do not delete shared cache namespaces without explicit approval.
- Do not force-push or modify shared environment templates unless requested.
- Preserve a known-good image/tag so environment testing can roll back.
- If a new design changes existing cache keys or paths, document that the first run after rollout will be cold; image rollback alone does not prove stored-cache compatibility.
- Keep risky or compatibility-breaking support default-off until the migration decision is approved.

## Examples

### New support: Cargo

Confirm workspace and lockfile behavior; cache verified dependency subsets such as registry indexes/packages and Git databases. Exclude `target`, toolchains, credentials, and the whole Cargo home. Prove registry and Git dependencies work after a clean restore in the same cross-container topology used by Harness.

### Existing support: Python virtual environments

Characterize the old `.venv`/`venv` path and archive behavior; retain manager download caches; remove non-portable environments; prove baseline/candidate migration behavior; and verify the candidate no longer produces predictable missing-path partial results.

## Completion criteria

The work is complete only when:

- requirements and design decisions are recorded;
- detection and cache paths are precise and idempotent;
- keys invalidate on dependency changes;
- focused tests pass;
- local cold/warm lifecycle succeeds;
- the exact pinned image succeeds in the target environment;
- warm restoration is proven by logs or file evidence;
- the real package manager is proven to consume restored data;
- legacy behavior and archive interoperability are verified or an explicit migration is approved;
- restore/build/save path visibility matches the production container topology;
- unexpected partial, missing, or failed operations are resolved or documented;
- feedback is incorporated into tests and implementation;
- documentation and final evidence are ready for review.
