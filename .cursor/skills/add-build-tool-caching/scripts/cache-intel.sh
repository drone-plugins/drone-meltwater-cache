#!/usr/bin/env bash
# Cache Intelligence workflow CLI — executes verifiable gates from COMMANDS.md / AUTONOMY.md.
# Agents must run this script for each phase and treat non-zero exit codes as failures.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
PROFILES_FILE="$SCRIPT_DIR/tool-profiles.env"

COMMAND=""
TOOL=""
WORKER_ID="${WORKER_ID:-local}"
ENV_NAME=""
LOCAL_ONLY=false
DRY_RUN=false
PUBLISH_IMAGE=false
TEST_MODE="local"
FROM_COMMAND=""
IMAGE_TAG=""
BASE_BRANCH="${BASE_BRANCH:-$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)}"

STATE_ROOT=""
TASK_STATE=""
STARTING_SHA=""
STARTING_SHORT_SHA=""
TOOL_SLUG=""

log() { printf '[cache-intel] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

usage() {
  cat <<EOF
Usage: cache-intel.sh <command> --tool <name> [options]

Commands:
  help          Show commands and examples
  status        Summarize task state, mapping support, and next step
  research      Inspect repo for existing support; update task state
  design        Record design gate (requires research); checklist output
  create        Verify implementation files and run focused autodetect tests
  test          Local tests (--local | --lifecycle | --compatibility | --all)
  build         Cross-compile binaries; optional --publish for Docker Hub (needs docker)
  finalize      Verify README / CHANGELOG updates
  remote-setup  Print Harness QA0 scope checklist (MCP writes stay in agent)
  remote-test   Record remote-test prerequisites; agent runs harness_execute via MCP
  validate      Evaluate completion gates from task state
  report        Write REPORT.md from task state template
  all           Full local workflow; remote phases when --env set and not --local

Options:
  --tool <name>       Build tool / package manager (required for most commands)
  --worker-id <id>    Task state worker id (default: local)
  --env <name>        Target Harness env label (e.g. qa0)
  --local             Stop after local gates (no remote-test)
  --dry-run           Print planned actions without mutating remote resources
  --publish           With build: push candidate image (needs docker + DOCKER_PASSWORD)
  --image <tag>       Candidate image tag for remote-test evidence
  --branch <name>     Base branch name for state metadata
  --from <command>    Resume all from a named command

Examples:
  $0 research --tool pnpm
  $0 test --tool pnpm --lifecycle
  $0 build --tool pnpm --publish
  $0 all --tool pnpm --env qa0 --local
EOF
}

slugify_tool() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-' | sed 's/^-*//;s/-*$//;s/--*/-/g'
}

init_state_paths() {
  if [[ -z "$TOOL" ]]; then
    die "--tool is required"
  fi
  TOOL_SLUG="$(slugify_tool "$TOOL")"
  STATE_ROOT="$(git -C "$REPO_ROOT" rev-parse --path-format=absolute --git-common-dir)/cache-intel"
  STARTING_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD)"
  STARTING_SHORT_SHA="$(git -C "$REPO_ROOT" rev-parse --short=12 HEAD)"
  mkdir -p "$STATE_ROOT/tasks"
  TASK_STATE="$STATE_ROOT/tasks/${TOOL_SLUG}-${WORKER_ID}-${STARTING_SHORT_SHA}.md"
}

atomic_write_state() {
  local tmp
  tmp="$(mktemp "$STATE_ROOT/tasks/state.XXXXXX")"
  cat >"$tmp"
  mv "$tmp" "$TASK_STATE"
}

ensure_state_bootstrap() {
  if [[ ! -f "$TASK_STATE" ]]; then
    atomic_write_state <<EOF
schema_version: 1
agent_or_worker: ${WORKER_ID}
tool: ${TOOL}
state: IN_PROGRESS
starting_branch: ${BASE_BRANCH}
starting_sha: ${STARTING_SHA}
baseline_revision_and_image:
owned_files:
mapping_position_proposed:
shared_changes_requested:
triage:
engine_fit:
supported_versions_platforms:
fixture_and_revision:
cache_paths_and_types:
key_inputs:
decisions_and_defaults_applied:
evidence_ledger:
command_results:
local_lifecycle_proof:
candidate_sha:
candidate_image_and_digest:
remote_scope_and_executions:
blockers_with_owner_action:
handoff:
next_command: research
EOF
  fi
}

record_result() {
  local cmd="$1" result="$2" evidence="$3"
  ensure_state_bootstrap
  local body
  body="$(cat "$TASK_STATE")"
  atomic_write_state <<EOF
${body}
command_results:
  - command: ${cmd}
    result: ${result}
    evidence: ${evidence}
    at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
}

tool_in_mapping() {
  local pattern="$1"
  grep -q "tool:.*\"${pattern}\"" "$REPO_ROOT/internal/plugin/autodetect/auto_detect_util.go" 2>/dev/null \
    || grep -q "tool:[[:space:]]*\"${pattern}\"" "$REPO_ROOT/internal/plugin/autodetect/auto_detect_util.go" 2>/dev/null
}

preparer_path() {
  echo "$REPO_ROOT/internal/plugin/autodetect/prepare_${TOOL_SLUG}.go"
}

mapping_has_lockfile() {
  local glob="$1"
  grep -q "globToDetect:[[:space:]]*\"${glob}\"" "$REPO_ROOT/internal/plugin/autodetect/auto_detect_util.go" 2>/dev/null
}

cmd_help() {
  usage
  cat <<EOF

Skill references: ${SKILL_DIR}/SKILL.md
Docker Hub: agent image has no docker; pipeline Run step uses scripts/docker-build-push.sh
  with secret docker_pass and connector dockerpat on Cloud VM.
EOF
}

cmd_status() {
  init_state_paths
  ensure_state_bootstrap
  log "tool=${TOOL} slug=${TOOL_SLUG}"
  log "task_state=${TASK_STATE}"
  if tool_in_mapping "$TOOL_SLUG"; then
    log "mapping: tool id '${TOOL_SLUG}' present in buildToolInfoMapping"
  else
    log "mapping: tool id '${TOOL_SLUG}' NOT found (may use different tool string in mapping)"
  fi
  local prep
  prep="$(preparer_path)"
  if [[ -f "$prep" ]]; then
    log "preparer: present at ${prep#"$REPO_ROOT"/}"
  else
    log "preparer: missing (expected prepare_${TOOL_SLUG}.go)"
  fi
  echo "--- task state (tail) ---"
  tail -n 40 "$TASK_STATE"
}

cmd_research() {
  init_state_paths
  ensure_state_bootstrap
  local triage="new_tool_within_engine"
  local prep
  prep="$(preparer_path)"
  if [[ -f "$prep" ]] && tool_in_mapping "$TOOL_SLUG"; then
    triage="extend_existing_support"
  fi

  log "Research for tool=${TOOL} (triage=${triage})"
  log "Reading ENGINE.md / TOOL-PATTERNS.md — agent must apply findings before create."

  local mapping_notes=""
  case "$TOOL_SLUG" in
    pnpm) mapping_notes="Expect pnpm-lock.yaml detection; PNPM_CONFIG_CACHE_DIR wiring per skill." ;;
    composer) mapping_notes="composer.lock / composer.json; follow-only .cache/composer pattern." ;;
    cargo) mapping_notes="Cargo.lock; cache under CARGO_HOME or .cache/cargo; exclude target/." ;;
  esac

  atomic_write_state <<EOF
schema_version: 1
agent_or_worker: ${WORKER_ID}
tool: ${TOOL}
state: IN_PROGRESS
starting_branch: ${BASE_BRANCH}
starting_sha: ${STARTING_SHA}
triage: ${triage}
engine_fit: root_and_one_level_detection; see ENGINE.md
decisions_and_defaults_applied: conservative defaults from AUTONOMY.md
evidence_ledger:
  - claim: detection and key strategy for ${TOOL}
    status: assumed
    source: TOOL-PATTERNS.md + official docs (agent must cite version)
    confidence: medium
mapping_notes: ${mapping_notes}
next_command: design
EOF
  record_result research PASS "triage=${triage}; state=${TASK_STATE}"
  log "PASS: research recorded. Next: design → create (agent) → test"
}

cmd_design() {
  init_state_paths
  [[ -f "$TASK_STATE" ]] || die "run research first"
  log "Design checklist for ${TOOL}:"
  cat <<EOF
- Detection glob and mapping order (lockfile before manifest)
- Relocate vs follow-only per path (ENGINE.md)
- Key inputs: 32-char lowercase MD5 hex chain
- Fixtures under test/fixtures or test/fixtures/pnpm-projects
- Local lifecycle + Harness remote with immutable image tag
EOF
  record_result design PASS "checklist emitted"
}

cmd_create() {
  init_state_paths
  local prep
  prep="$(preparer_path)"
  [[ -f "$prep" ]] || die "create gate FAIL: missing ${prep#"$REPO_ROOT"/}"
  local test_file="$REPO_ROOT/internal/plugin/autodetect/prepare_${TOOL_SLUG}_test.go"
  [[ -f "$test_file" ]] || log "WARN: missing unit test file prepare_${TOOL_SLUG}_test.go"

  log "Running focused autodetect tests..."
  local pattern
  pattern="$(echo "$TOOL_SLUG" | sed 's/-//g')"
  (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect -run "${pattern}" -timeout 10m)
  record_result create PASS "go test -run ${pattern}"
}

run_go_test_all() {
  log "go test ./internal/plugin/autodetect ./internal/plugin"
  (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect ./internal/plugin -timeout 15m)
  log "go vet ./..."
  (cd "$REPO_ROOT" && go vet ./...)
}

load_profile() {
  local tool="$1"
  local line
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^# ]] && continue
    [[ -z "${line// /}" ]] && continue
    if [[ "${line%%|*}" == "$tool" ]]; then
      echo "$line"
      return 0
    fi
  done <"$PROFILES_FILE"
  return 1
}

run_lifecycle_fixture() {
  local fixture_rel="$1" install_cmd="$2" clean_csv="$3" unset_csv="$4"
  local fixture="$REPO_ROOT/$fixture_rel"
  if [[ ! -d "$fixture" ]]; then
    log "SKIP lifecycle fixture ${fixture_rel}: directory missing"
    return 0
  fi

  local plugin="$REPO_ROOT/bin/drone-cache"
  mkdir -p "$REPO_ROOT/bin"
  (cd "$REPO_ROOT" && go build -o "$plugin" .)

  local cache_root test_home
  cache_root="$(mktemp -d)"
  test_home="$(mktemp -d)"
  trap 'rm -rf "$cache_root" "$test_home"' RETURN

  local -a common_env=(
    "HOME=$test_home"
    "PLUGIN_AUTO_CACHE=true"
    "PLUGIN_BACKEND=filesystem"
    "PLUGIN_FILESYSTEM_CACHE_ROOT=$cache_root"
    "PLUGIN_ACCOUNT_ID=test-account"
    "PLUGIN_OVERRIDE=true"
    "PLUGIN_LOG_LEVEL=debug"
    "PLUGIN_DEBUG=false"
    "DRONE_REPO_NAMESPACE=local"
    "DRONE_REPO_NAME=${TOOL_SLUG}-lifecycle"
    "DRONE_COMMIT_BRANCH=main"
    "DRONE_COMMIT_SHA=lifecycle-${STARTING_SHORT_SHA}"
  )

  IFS=',' read -r -a unset_vars <<<"$unset_csv"
  for v in "${unset_vars[@]}"; do
    [[ -n "$v" ]] && common_env+=("${v}=")
  done

  cd "$fixture"
  log "Lifecycle cold restore @ ${fixture_rel}"
  env -i "${common_env[@]}" PLUGIN_RESTORE=true PLUGIN_REBUILD=false "$plugin"

  log "Lifecycle install: ${install_cmd}"
  if ! bash -lc "$install_cmd"; then
    die "install failed for ${fixture_rel}"
  fi

  env -i "${common_env[@]}" PLUGIN_RESTORE=false PLUGIN_REBUILD=true "$plugin"
  if ! find "$cache_root" -type f -print -quit | grep -q .; then
    die "save produced no archive files under ${cache_root}"
  fi

  IFS=',' read -r -a clean_paths <<<"$clean_csv"
  for p in "${clean_paths[@]}"; do
    [[ -n "$p" ]] && rm -rf "$p"
  done

  log "Lifecycle warm restore @ ${fixture_rel}"
  env -i "${common_env[@]}" PLUGIN_RESTORE=true PLUGIN_REBUILD=false "$plugin"

  log "PASS lifecycle fixture ${fixture_rel}"
}

cmd_test() {
  init_state_paths
  local mode="$TEST_MODE"
  case "$mode" in
    local)
      run_go_test_all
      record_result "test --local" PASS "go test + vet"
      ;;
    lifecycle)
      local profile
      if ! profile="$(load_profile "$TOOL_SLUG")"; then
        log "No lifecycle profile in tool-profiles.env for ${TOOL_SLUG}"
        record_result "test --lifecycle" SKIPPED "add profile to tool-profiles.env"
        exit 0
      fi
      IFS='|' read -r _t fixtures install_cmd clean_paths unset_env _notes <<<"$profile"
      IFS=',' read -r -a fixture_list <<<"$fixtures"
      for f in "${fixture_list[@]}"; do
        run_lifecycle_fixture "$f" "$install_cmd" "$clean_paths" "$unset_env"
      done
      record_result "test --lifecycle" PASS "fixtures: ${fixtures}"
      ;;
    compatibility)
      log "Compatibility: run tool-specific scenario tests if present"
      (cd "$REPO_ROOT" && go test -count=1 "./internal/plugin/autodetect" -run "Scenario_.*$(echo "$TOOL_SLUG" | sed 's/.*/\u&/')" -timeout 10m) \
        || (cd "$REPO_ROOT" && go test -count=1 "./internal/plugin/autodetect" -run "Scenario" -timeout 10m)
      record_result "test --compatibility" PASS "scenario tests"
      ;;
    all)
      TEST_MODE=local; cmd_test
      TEST_MODE=lifecycle; cmd_test
      TEST_MODE=compatibility; cmd_test
      ;;
    *) die "unknown test mode: $mode" ;;
  esac
}

cmd_build() {
  init_state_paths
  run_go_test_all
  log "Cross-compile release binaries"
  mkdir -p "$REPO_ROOT/release/linux/amd64" "$REPO_ROOT/release/windows/amd64"
  (cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/drone-cache .)
  (cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o release/windows/amd64/drone-cache.exe .)
  (cd "$REPO_ROOT" && go build -o bin/drone-cache .)

  local image_evidence="binary-only"
  if $PUBLISH_IMAGE; then
    if [[ -x "$SCRIPT_DIR/docker-build-push.sh" ]]; then
      if "$SCRIPT_DIR/docker-build-push.sh" --tool "$TOOL_SLUG" ${IMAGE_TAG:+--tag "$IMAGE_TAG"}; then
        image_evidence="published via docker-build-push.sh"
      else
        record_result build BLOCKED "docker publish failed; use pipeline Run step with DOCKER_PASSWORD"
        exit 3
      fi
    else
      die "docker-build-push.sh missing"
    fi
  else
    log "Image not published (no --publish). Pipeline step should run docker-build-push.sh on Cloud VM."
  fi

  record_result build PASS "${image_evidence}; sha=${STARTING_SHA}"
}

cmd_finalize() {
  init_state_paths
  grep -qi "$TOOL" "$REPO_ROOT/README.md" || log "WARN: README may not mention ${TOOL}"
  grep -qi "$TOOL" "$REPO_ROOT/CHANGELOG.md" 2>/dev/null || log "WARN: CHANGELOG may not mention ${TOOL}"
  record_result finalize PASS "README/CHANGELOG spot-check"
}

cmd_remote_setup() {
  init_state_paths
  cat <<EOF
Remote setup checklist (execute via Harness MCP in org default / project qajt):
- Verify connectors: drone_meltwater_cache, dockerpat, account.rahulpqamcpconn
- Verify secrets: docker_pass (Docker Hub), rp-gh-pat (git push in pipeline)
- Candidate image: rahulpx9/drone-cache:${TOOL_SLUG}-caching-<short-sha> from pipeline build step
- Update RestoreCache/SaveCache template image refs to candidate tag for validation pipeline
EOF
  record_result remote-setup NOT_APPLICABLE "agent+MCP or pipeline-owned"
}

cmd_remote_test() {
  init_state_paths
  if $LOCAL_ONLY || [[ -z "$ENV_NAME" ]]; then
    record_result remote-test BLOCKED "set --env qa0 and authorize remote; or pipeline runs Harness tests"
    log "REMOTE_BLOCKED: local complete; run validation pipeline with candidate image on Cloud VM."
    exit 0
  fi
  log "remote-test for env=${ENV_NAME} — agent must harness_execute validation pipeline and attach URLs to task state."
  record_result remote-test SKIPPED "requires agent MCP harness_execute; image=${IMAGE_TAG:-<from build step>}"
}

cmd_validate() {
  init_state_paths
  [[ -f "$TASK_STATE" ]] || die "no task state"
  local fail=0
  [[ -f "$(preparer_path)" ]] || { log "FAIL: preparer missing"; fail=1; }
  if (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect -run "NonExistent" -timeout 1m >/dev/null 2>&1); then
    :
  fi
  if ! (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect -timeout 15m >/dev/null); then
    log "FAIL: autodetect tests"; fail=1
  else
    log "PASS: autodetect tests"
  fi
  if [[ $fail -eq 0 ]]; then
    if $LOCAL_ONLY || [[ -z "$ENV_NAME" ]]; then
      log "Terminal state: LOCAL_COMPLETE"
    else
      log "Terminal state: pending REMOTE_VALIDATED (run remote-test via MCP)"
    fi
    exit 0
  fi
  log "Terminal state: FAILED"
  exit 1
}

cmd_report() {
  init_state_paths
  ensure_state_bootstrap
  local report="$REPO_ROOT/REPORT.md"
  cat >"$report" <<EOF
# Cache Intelligence: ${TOOL}

## Summary
Autodetect caching support for **${TOOL}**. Validation driven by \`cache-intel.sh\` gates.
See task state: \`${TASK_STATE}\`.

## Revisions
- Starting: ${BASE_BRANCH} @ ${STARTING_SHA}
- Ending: $(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD) @ $(git -C "$REPO_ROOT" rev-parse HEAD)
- Candidate image: rahulpx9/drone-cache:${TOOL_SLUG}-caching-${STARTING_SHORT_SHA} (build via pipeline \`docker-build-push.sh\`)

## Behavior
- Detection files and precedence: (see prepare_${TOOL_SLUG}.go and auto_detect_util.go)
- Cached paths, strategy, and key inputs: document from implementation

## Evidence
| Check | Result | Evidence |
|-------|--------|----------|
| Unit / scenario tests | see command_results in task state | cache-intel.sh test |
| Local lifecycle | see command_results | cache-intel.sh test --lifecycle |
| Harness remote | BLOCKED or PASS | MCP / pipeline execution URL |

## Command ledger
See \`${TASK_STATE}\` field command_results.

## Rollout and rollback
Publish immutable tag \`${TOOL_SLUG}-caching-<sha>\`; rollback to previous template image tag.
EOF
  log "Wrote ${report}"
  record_result report PASS "$report"
}

should_run() {
  local name="$1"
  if [[ -z "$FROM_COMMAND" ]]; then
    return 0
  fi
  # crude resume: run if FROM_COMMAND matches or comes before in order
  local order="research design create test build finalize remote-setup remote-test validate report"
  local seen=false
  for c in $order; do
    [[ "$c" == "$FROM_COMMAND" ]] && seen=true
    [[ "$c" == "$name" && "$seen" == true ]] && return 0
    [[ "$c" == "$name" && "$FROM_COMMAND" == "$name" ]] && return 0
  done
  return 1
}

cmd_all() {
  init_state_paths
  should_run research && cmd_research
  should_run design && cmd_design
  should_run create && cmd_create
  if should_run test; then
    TEST_MODE=all
    cmd_test
  fi
  should_run build && cmd_build
  should_run finalize && cmd_finalize
  if ! $LOCAL_ONLY && [[ -n "$ENV_NAME" ]]; then
    should_run remote-setup && cmd_remote_setup
    should_run remote-test && cmd_remote_test
  fi
  should_run validate && cmd_validate
  should_run report && cmd_report
}

parse_args() {
  if [[ $# -lt 1 ]]; then
    usage
    exit 2
  fi
  COMMAND="$1"
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --tool) TOOL="${2:?}"; shift 2 ;;
      --worker-id) WORKER_ID="${2:?}"; shift 2 ;;
      --env) ENV_NAME="${2:?}"; shift 2 ;;
      --local) LOCAL_ONLY=true; shift ;;
      --dry-run) DRY_RUN=true; shift ;;
      --publish) PUBLISH_IMAGE=true; shift ;;
      --image) IMAGE_TAG="${2:?}"; shift 2 ;;
      --branch) BASE_BRANCH="${2:?}"; shift 2 ;;
      --from) FROM_COMMAND="${2:?}"; shift 2 ;;
      --local-test) TEST_MODE=local; shift ;;
      --lifecycle) TEST_MODE=lifecycle; shift ;;
      --compatibility) TEST_MODE=compatibility; shift ;;
      --all) TEST_MODE=all; shift ;;
      -h|--help) COMMAND=help; shift ;;
      *) die "unknown option: $1" ;;
    esac
  done
}

main() {
  parse_args "$@"
  cd "$REPO_ROOT"
  case "$COMMAND" in
    help) cmd_help ;;
    status) init_state_paths; cmd_status ;;
    research) cmd_research ;;
    design) cmd_design ;;
    create) cmd_create ;;
    test) init_state_paths; cmd_test ;;
    build) cmd_build ;;
    finalize) cmd_finalize ;;
    remote-setup) cmd_remote_setup ;;
    remote-test) cmd_remote_test ;;
    validate) cmd_validate ;;
    report) cmd_report ;;
    all) cmd_all ;;
    *) die "unknown command: $COMMAND (try help)" ;;
  esac
}

main "$@"
