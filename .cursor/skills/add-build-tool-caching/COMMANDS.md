# Command Reference

Use this file with `SKILL.md`. Commands are workflow selectors, not shell commands. Interpret a request such as:

```text
/add-build-tool-caching research --tool uv
/add-build-tool-caching create --tool cargo
/add-build-tool-caching remote-test --env qa0 --pipeline <id>
/add-build-tool-caching all --tool pnpm --env qa0
```

Equivalent natural-language requests are valid. Never require the user to use exact syntax.

## Common arguments

Infer arguments from conversation, repository, saved command state, and environment before asking.

```text
--language <name>       Language ecosystem
--tool <name>           Package manager or build tool
--manager <name>        Alias for --tool
--repo <path|url>       Source repository or fixture
--branch <name>         Source branch
--env <name|url>        Target environment or Harness URL
--account <id>          Harness account
--org <id>              Harness organization
--project <id>          Harness project
--pipeline <id|url>     Harness pipeline
--execution <id|url>    Harness execution
--image <tag|digest>    Candidate plugin image
--baseline <tag|sha>    Baseline revision or image
--mode <v0|v1>          Pipeline generation/runtime mode
--from <command>        Resume from a named command
--dry-run                Produce design/payloads without writes
```

Ask only for a missing value that materially blocks or changes the selected operation. Harness write operations require known organization and project scope unless the resource is intentionally account-scoped.

## Command routing

Supported commands:

- `help`: show commands, expected arguments, and examples.
- `status`: summarize completed evidence, current phase, blockers, and next command.
- `research`: gather requirements and verify build-tool behavior.
- `design`: produce detection, path, key, compatibility, and test design.
- `create`: implement the smallest complete code-and-test increment.
- `test`: run local unit, scenario, regression, and cache-lifecycle tests.
- `build`: build the candidate binary/image and record immutable identity.
- `remote-setup`: discover or create Harness validation resources.
- `remote-test`: execute cold, warm, invalidation, and compatibility runs.
- `diagnose`: investigate a local or Harness test failure.
- `validate`: evaluate all acceptance and compatibility gates.
- `report`: produce implementation, evidence, rollout, and limitation reports.
- `all`: run the complete workflow in dependency order.

If no command is supplied:

1. infer the narrowest command from the request;
2. use `status` when asked what remains;
3. use `all` only when the user clearly requests end-to-end implementation;
4. state the selected command before performing material work.

Commands may be combined:

```text
research + design
create + test
build + remote-test
validate + report
```

Do not expand a narrow command into `all`.

## Command state

Maintain this state in the conversation or a user-requested implementation log:

```text
language:
tool:
starting_sha:
baseline_image:
candidate_sha:
candidate_image:
fixture_repo:
local_results:
harness_scope:
pipeline:
cold_execution:
warm_execution:
invalidation_execution:
compatibility_results:
decisions:
blockers:
next_command:
```

Do not invent missing state. Verify mutable remote state before using it.

## `help`

Return:

1. the command list;
2. required arguments for the requested operation;
3. two or three relevant invocation examples;
4. the recommended next command, if context exists.

Do not inspect or mutate code or remote resources for `help`.

## `status`

Inspect available local and remote evidence without changing it. Report:

- selected tool and intended support;
- baseline and candidate identities;
- completed commands and their outcomes;
- local test status;
- Harness pipeline/execution status when known;
- unresolved design decisions and blockers;
- one recommended next command.

Use Harness read tools when execution state may have changed.

## `research`

Follow `SKILL.md` sections 1 and 2.

Outputs:

- requirements record;
- primary documentation findings;
- detection and key candidates;
- cache-path classification;
- runtime/container-sharing constraints;
- security and portability exclusions;
- unanswered questions requiring user feedback.

Research must distinguish documented behavior, observed behavior, and assumptions. Do not edit production code.

## `design`

Follow `SKILL.md` section 3.

Outputs:

- tool identity and detection rules;
- root/nested/workspace discovery behavior;
- coexistence and ambiguity rules;
- path/configuration precedence;
- key inputs and migration effects;
- restore/save behavior;
- scenario and compatibility matrices;
- local and remote validation plan.

Stop for feedback when valid alternatives affect backward compatibility, portability, security, or rollout.

## `create`

Follow `SKILL.md` sections 4 and 5.

Preconditions:

- requirements and design are known;
- ambiguous compatibility decisions are resolved;
- starting revision and working-tree state are recorded.

Actions:

1. implement one reviewable ecosystem or behavior increment;
2. add unit, scenario, and regression tests with the code;
3. preserve unrelated changes;
4. format changed files;
5. run focused tests;
6. return changed files, behavior, tests, and remaining work.

`create` does not publish an image or mutate Harness resources unless combined with the corresponding command.

## `test`

Follow `SKILL.md` sections 5 and 6.

Modes:

- `test --local`: focused and full repository tests;
- `test --lifecycle`: local restore/install/save/clean/restore/install;
- `test --compatibility`: baseline/candidate keys, side effects, and archive interoperability;
- `test --all`: all applicable local modes.

Use direct commands with propagated exit codes. Report every required command as passed, failed, or skipped with a reason. A wrapper command that ignores failures is not evidence.

## `build`

Actions:

1. verify local gates required before image creation;
2. build supported binary/container targets using repository conventions;
3. smoke-test the candidate locally;
4. tag with branch/commit identity;
5. publish only when requested or required for an approved remote test;
6. record tag and digest;
7. preserve the known-good baseline identity.

Never use `latest` as validation evidence.

## `remote-setup`

Use Harness MCP when available. This command may discover, verify, create, or update validation resources.

Required sequence:

1. Select the MCP connection matching `--env`; do not silently use production for QA or vice versa.
2. Establish account, organization, and project from explicit arguments, a Harness URL, or confirmed context.
3. Use `harness_describe` to confirm resource types and operations when uncertain.
4. Use `harness_list`/`harness_get` to verify repositories, connectors, secrets, delegates/infrastructure, templates, and pipelines.
5. Reuse suitable resources. Do not create duplicates.
6. Use `harness_schema` before constructing an unfamiliar create/update body.
7. Preview the complete write payload in chat.
8. Use `harness_create` or `harness_update`; rely on the MCP server's approval dialog instead of asking for a second confirmation.
9. Re-read the resource and verify the candidate image, repository, branch, cache settings, and pipeline version.

Prefer a dedicated validation pipeline. Keep baseline and candidate pipelines/runs distinguishable. Do not modify shared production resources without explicit authorization.

## `remote-test`

Use Harness MCP for execution and monitoring.

Preconditions:

- candidate image/tag is immutable and reachable;
- Harness scope and pipeline are verified;
- the pipeline actually references the candidate;
- fixture repo, branch, install command, expected path, and key inputs are known.

Required sequence:

1. Preview pipeline, inputs, image, expected cache paths, and test matrix.
2. Use `harness_execute` to run the pipeline.
3. Poll with `harness_get` or execution-list operations until terminal status.
4. Run cold, warm, invalidation, and requested compatibility cases.
5. Use `harness_diagnose` for completed failures and relevant execution summaries/logs.
6. Capture execution URLs, statuses, restore/save results, tool-consumption evidence, timing, and bytes.
7. If MCP cannot access logs, report that limitation and request only the precise log markers needed from the user.

Do not clear or delete shared caches. Use unique keys/namespaces for cold tests unless explicit deletion is approved. A green execution, optimization label, or restored dummy file does not prove the package manager consumed cached data.

## `diagnose`

For local failures:

1. reproduce with the narrowest direct command;
2. classify code regression, fixture issue, environment dependency, or pre-existing failure;
3. inspect relevant logs and code;
4. add a regression test before fixing an implementation defect;
5. rerun the narrow test, then affected broader gates.

For Harness failures:

1. verify execution, pipeline, image, inputs, and environment;
2. use `harness_get` and `harness_diagnose`;
3. distinguish cache miss from permissions, backend, archive, network, infrastructure, and install failures;
4. correct local code or approved remote configuration;
5. rerun only affected cases before the full matrix.

Do not remediate access-denied, quota, entitlement, or authentication failures by guessing alternate scope.

## `validate`

Evaluate completion criteria from `SKILL.md` without making unrelated changes.

Return each gate as:

```text
PASS: evidence
FAIL: evidence and required correction
BLOCKED: exact dependency and owner/action
NOT APPLICABLE: reason
```

Never claim all-platform, zero-regression, or end-to-end support from unit tests alone.

## `report`

Use the final report structure in `SKILL.md`. Include links for Harness executions and other remote resources. Separate:

- implemented behavior;
- measured evidence;
- compatibility/migration effects;
- environment limitations;
- unresolved decisions;
- staged rollout and rollback guidance.

Do not convert an unverified candidate path into a supported-tool claim.

## `all`

Run commands in this order:

```text
research
design
create
test --all
build
remote-setup
remote-test
validate
report
```

Rules:

- stop at a user-owned design decision;
- retry transient technical failures or use a safe alternative;
- continue independent work when one external dependency is blocked;
- do not skip a failed gate;
- after each command, update state and incorporate feedback;
- do not perform remote writes, image publication, or deployment unless needed for the requested end-to-end scope and authorized through the relevant approval mechanism.

## Harness MCP tool map

Use the consolidated Harness tools:

- discovery: `harness_describe`, `harness_schema`, `harness_search`;
- reads: `harness_list`, `harness_get`, `harness_status`;
- writes: `harness_create`, `harness_update`, `harness_delete`;
- execution: `harness_execute`;
- failure analysis: `harness_diagnose`.

Use `harness_delete` only when the user explicitly requests cleanup and the exact resource is verified. Surface governance or policy failures as blockers and adjust the proposed configuration before retrying.
