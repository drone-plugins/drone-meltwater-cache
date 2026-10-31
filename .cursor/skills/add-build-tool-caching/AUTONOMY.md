# Autonomous Execution and Multi-Agent Coordination

Use this policy for implementation requests and parallel tool work. The goal is to complete all safe, reversible work without pausing for routine choices while preserving explicit approval for compatibility, security, shared-resource, and external-write decisions.

## Default implementation route

When the user asks to **add**, **implement**, or **support** caching for a tool without naming a command, run:

```text
research → design → create → test --all → build → finalize → harness-core → validate → report
```

`test --all` includes the public-project lifecycle. Run `remote-setup` and `remote-test` only when the user supplied or previously authorized the environment, scope, publication, and execution. Missing remote access does not block local completion. `harness-core` needs a harness-core checkout; without one it records BLOCKED with the owner and continues.

An explicit narrow command remains narrow. For example, `research` and `diagnose` never edit production code. Combine `diagnose + create` when the user requests diagnosis and correction.

## Conservative decision defaults

Apply these in order and record the decision instead of asking:

1. Limit scope to the assigned tool and existing engine capabilities.
2. Prefer a package/download/content-addressed cache that the tool validates.
3. Exclude build outputs, toolchains, credentials, daemon state, and ABI-coupled content by default. Include an installed tree only when direct lifecycle evidence proves archive fidelity, lockfile invalidation, portability across the claimed support matrix, and clear value over restore/save overhead; otherwise exclude it without asking.
4. Prefer a committed lockfile as the detection and key input. Use a manifest fallback only for ecosystems where lockless projects are normal.
5. Preserve existing mapping order. Propose the new entry immediately before the generic fallback it should supersede, otherwise after related tools; characterize mixed-tool key effects.
6. Honor explicit environment configuration, then existing native project configuration.
7. Prefer **follow only** when the selected path is already shared across steps. Relocate only when the tool reliably discovers the generated project configuration in later containers without hidden wiring.
8. Never rewrite tracked configuration solely to avoid a harmless cold miss.
9. Support only versions, operating systems, architectures, and runtime combinations proven by fixtures, current release targets, or direct tests. Do not infer broad support from cross-compilation.
10. Use a small deterministic open-source fixture with a committed lockfile. If no suitable fixture exists, create the smallest isolated fixture and state that it is synthetic.
11. Treat a cache as worthwhile only when measured restore/save overhead is meaningfully lower than the saved install work. Report a recommendation not to cache when it is not.
12. Do not broaden into an engine change to rescue an unsupported edge case. Complete the safe subset and record the edge case separately.

## Interruption policy

Continue autonomously for additive, reversible choices covered by the defaults above. Retry transient failures, use safe alternatives, and continue independent work.

Ask the user only when at least one condition applies:

- an engine-wide change can alter existing detection, keys, path order, restore/save coordination, or archive behavior;
- an existing tool's key or path will migrate and no backward-compatible option exists;
- two materially different product behaviors remain after applying the defaults;
- a design may archive credentials, execute unverified cached content, or accept a security regression;
- work requires modifying shared templates, default images, production resources, or another agent's owned files;
- a destructive action, cache deletion, force push, merge, release, deployment, registry publication, or unapproved external write is required;
- a material limitation must be accepted as supported behavior rather than documented as unsupported.

Batch all current user-owned decisions into one request. Put the recommended option first and include consequences. Do not ask about fixture selection, routine naming, safe cache exclusions, local test strategy, or unavailable remote access.

Native approval dialogs for Harness writes/executions and other protected external actions are unavoidable. Preview the payload once in chat, then rely on the native approval instead of asking twice.

## Terminal states

Every task ends in one of these states:

- `RESEARCHED`: evidence and recommendation complete; no implementation requested.
- `DESIGNED`: implementation-ready design complete; blocked only by a user-owned decision.
- `LOCAL_COMPLETE`: implementation, focused/full applicable tests, local lifecycle, docs, and report complete.
- `REMOTE_BLOCKED`: local work is complete; an exact remote dependency, owner, and action are recorded.
- `REMOTE_VALIDATED`: the immutable candidate passed target-environment cold, warm, and invalidation runs (`remote-cold`, `remote-warm`, `remote-invalidate` all PASS), plus applicable compatibility cases. Product-path runs (`product-seed`, `product-warm`) are additional evidence, not a requirement.
- `INTEGRATION_BLOCKED`: tool-local work is complete but awaits coordinator-owned shared changes or conflict resolution.
- `FAILED`: a reproducible correctness or safety gate failed and no safe correction remains in scope.

Do not label authentication, authorization, quota, entitlement, missing scope, or unavailable infrastructure as an implementation failure. Use `REMOTE_BLOCKED` and stop retrying after confirming the denial once.

## Durable task state

Do not rely only on conversation memory. `cache-intel.sh` keeps task state outside the working tree, so it is shared by linked worktrees, survives commits, and never enters the product diff:

```text
$(git rev-parse --git-common-dir)/cache-intel/
  work/<tool>-<worker>/        one directory per task; only that worker writes it
    state.env                  tool, worker, base branch, starting SHA, candidate image, terminal state
    research.md, design.md     the research and design records (the research Fields list drives later gates)
    ledger.tsv                 every gate result, oldest first: time, gate, result, evidence
    metrics.env                measured local values (fixture SHA, timings, archive size, keys)
    evidence/                  logs for each gate, archive listings
    remote/                    rendered pipelines, runs.tsv (one row per Harness run), fixture-commit.json
    out/                       PR_BODY.md, HARNESS_CORE_PR.md, harness-core.env/.patch, summary.env
  harness-core/                sparse harness-core clone when the orchestrator provides one
```

`TOOL` and `WORKER_ID` must match `^[a-z0-9][a-z0-9.-]*$`; the CLI rejects anything else. A standalone agent uses worker ID `local`. Read state with `cache-intel.sh status`; never edit `ledger.tsv` or `runs.tsv` by hand. Record an external blocker with `cache-intel.sh waive --gate <gate> --result BLOCKED --reason "<owner and action>"`.

Never put credentials, tokens, raw private configuration, or secret-bearing logs in task state, evidence, or reports.

## Evidence ledger

For every design-critical claim, record:

```text
claim:
status: documented | observed | assumed
source: official URL/file and applicable version, or command
environment: tool/runtime/image/OS/architecture
result:
confidence: high | medium | low
artifact_or_log:
```

Assumptions may guide experiments but cannot become supported behavior without observed or authoritative evidence. The final report must preserve every required command as `PASS`, `FAIL`, `SKIPPED`, `BLOCKED`, or `NOT_APPLICABLE`.

## Parallel-agent protocol

### Coordinator responsibilities

Before launching workers:

1. assign one tool identifier and isolated branch/worktree per worker;
2. assign a stable worker ID and starting SHA;
3. reserve ownership and the proposed mapping position for each worker;
4. identify shared helpers, likely engine changes, and conflicting ecosystems;
5. assign collision-free names using `<tool>-<worker-id>-<starting-short-sha>` for branches, images, fixtures, pipelines, and cache namespaces.

The coordinator exclusively owns:

- final `buildToolInfoMapping` insertion and ordering;
- engine-wide behavior and shared helper changes;
- mixed-tool key/order compatibility tests;
- shared README/CHANGELOG integration;
- shared image tags, Harness resources, and remote namespaces;
- final rebase, integration tests, and report consolidation.

### Worker responsibilities

Workers own tool-local research, preparer/config code, unit tests, scenario tests, fixtures, lifecycle evidence, and their mapping entry. Two modes exist:

- **Harness pipeline workers** (`tempai`): each worker has its own clone, branch `feat/<tool>-caching-<worker>`, image tag, validation pipeline, and pull request. The worker commits its `buildToolInfoMapping` entry as one isolated hunk, at a position chosen by the design record; if two worker PRs touch neighbouring lines, the second rebases at review time. This is the only mode in which a worker edits the mapping file.
- **Linked worktrees in one checkout** (several local agents): the coordinator owns the mapping file. Workers keep the proposed entry in `design.md`, apply it only inside their own worktree for tests, and finish as `INTEGRATION_BLOCKED` when only coordinator work remains.

In both modes workers must:

- avoid editing files assigned to another worker;
- request engine or shared-helper changes through the design record instead of implementing them silently;
- continue all independent work when a shared change is pending;
- never publish, deploy, merge, or mutate shared remote resources unless specifically assigned and approved. The orchestration pipeline, not the worker, pushes branches, publishes images, toggles the account-level Cache Intelligence image, and opens pull requests.

### Integration order

The coordinator integrates serially:

1. rebase each tool branch onto the selected base;
2. integrate shared helpers or approved engine changes;
3. apply worker-provided mapping patches in reserved order;
4. run focused tool tests, then mixed-tool detection/key/path-order tests;
5. integrate documentation and changelog entries;
6. run repository regression, race, vet, and supported builds;
7. build uniquely tagged candidates;
8. run remote validation in isolated namespaces where authorized;
9. consolidate each worker's `REPORT.md` evidence into the final report.

If workers discover the same engine limitation, create one coordinator-owned engine task. Tool workers must not independently implement competing engine changes.

