#!/usr/bin/env bash
# Cache Intelligence workflow CLI. Every command is a gate: a non-zero exit code means the gate
# did not pass. Fix the cause (or fill the requested document) and re-run the same command.
set -euo pipefail
((BASH_VERSINFO[0] >= 5)) || { echo "cache-intel.sh needs bash 5+ (found ${BASH_VERSION})" >&2; exit 1; }
shopt -u patsub_replacement 2>/dev/null || true

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
HELPERS="$SCRIPT_DIR/ci_helpers.py"
PROFILES_FILE="$SCRIPT_DIR/tool-profiles.env"
AUTODETECT_DIR="$REPO_ROOT/internal/plugin/autodetect"

COMMAND=""
TOOL=""
WORKER_ID="${WORKER_ID:-local}"
ENV_NAME=""
LOCAL_ONLY=false
PUBLISH_IMAGE=false
TEST_MODE="local"
IMAGE_TAG="${CANDIDATE_IMAGE:-}"
HARNESS_ORG="${HARNESS_ORG:-default}"
HARNESS_PROJECT="${HARNESS_PROJECT:-qajt}"
DOCKER_CONNECTOR="${DOCKER_CONNECTOR:-rpdockerhub}"
REPO_NAME="${REPO_NAME:-drone-plugins/drone-meltwater-cache}"
BASE_BRANCH="${BASE_BRANCH:-}"
RUNNER="${CACHE_INTEL_RUNNER:-auto}"
PRODUCT=false
HC_DIR=""
JIRA_ID="${JIRA_ID:-}"
HC_WINDOWS=true
REMOTE_RUN=""
EXECUTION_URL=""
REMOTE_RESULT=""
RUN_METRICS=""
GATE=""
REASON=""
FROM_DIR=""
FIXTURE_FILES=""
declare -A OPT=()

STATE_ROOT=""
WORK_DIR=""
TOOL_SLUG=""
declare -A F=()

log() { printf '[cache-intel] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required on PATH"; }

usage() {
  cat <<'EOF'
Usage: cache-intel.sh <command> --tool <name> --worker-id <id> [options]

Workflow (each command is a gate; non-zero exit = not passed):
  research       Scaffold, then validate, the research record (cache layout, key, CI wiring, fixture, sources)
  design         Scaffold, then validate, the design record (mapping entry, paths, scenario matrix)
  create         Check the preparer and mapping entry exist and run the tool's unit tests
  test           --local-test | --compatibility | --lifecycle | --public | --all
  build          Cross-compile release binaries (image is built by the orchestration pipeline)
  finalize       Require README and CHANGELOG entries for the tool
  harness-core   Add the tool's cache env to harness-core (VM + K8s + tests) when research says it is needed
  remote-setup   Render this worker's Harness validation pipeline(s) from the research fields
  remote-test    Record a Harness run: --run cold|warm|invalidate|product-seed|product-warm
  validate       Evaluate every gate and print the terminal state
  report         Generate REPORT.md, the GitHub PR body and the harness-core PR body
  all            research → design → create → test --all → build → finalize → harness-core → validate → report

Utilities:
  help | status
  waive          Record a gate as BLOCKED or NOT_APPLICABLE: --gate <name> --result BLOCKED --reason "<owner/action>"
  fixture-commit Build a Harness Code commit body from a directory: --from <dir> [--files a,b]

Common options:
  --tool <name>            build tool (required)          --worker-id <id>   isolates state, branch, image, pipeline
  --branch <name>          base branch for state metadata  --image <repo:tag> candidate image
  --env <name>             Harness env label (validate: require remote gates)
  --org / --project        Harness scope (default default / qajt)
  --docker-connector <id>  connector used to pull images (default rpdockerhub)
  --runner auto|host|docker   test --public: run the install on the host or in the research install_image

Research-field overrides (normally read from research.md):
  --fixture-repo --fixture-ref --fixture-subdir --install-image --install-cmd --offline-cmd
  --warm-check --invalidate-file --cache-env KEY=dir --harness-code-repo <id> --harness-code-branch <b>

remote-setup:   --product                 also render the product-path pipeline (needs harness_code_repo)
                --fixture-source github|harness_code   where the direct pipeline gets the fixture (default github)
remote-test:    --run <name> --execution-url <url> --result PASS|FAIL|BLOCKED [--metrics k=v,k=v] [--reason ...]
harness-core:   --hc-dir <checkout> (default <git-common-dir>/cache-intel/harness-core) [--jira CI-123] [--no-windows]

Examples:
  cache-intel.sh research --tool cargo --worker-id w1 --branch feat/php-caching-support
  cache-intel.sh test --tool cargo --worker-id w1 --public --runner docker
  cache-intel.sh remote-setup --tool cargo --worker-id w1 --image rahulpx9/drone-cache:cargo-w1-31
  cache-intel.sh remote-test --tool cargo --worker-id w1 --run warm --execution-url <url> --result PASS \
      --metrics install_seconds=0.41,cache_kb=16812
  cache-intel.sh validate --tool cargo --worker-id w1 --env qa0
EOF
}

# ------------------------------------------------------------------------ state

slugify() { echo "$1" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-' | sed 's/^-*//;s/-*$//'; }
clean_value() { printf '%s' "$1" | tr '\t\n\r' '   '; }

kv_get() {
  local file="$1" key="$2" line
  [[ -f "$file" ]] || return 0
  while IFS= read -r line; do
    [[ "${line%%=*}" == "$key" ]] && { printf '%s' "${line#*=}"; return 0; }
  done <"$file"
  return 0
}

kv_set() {
  local file="$1" key="$2" value tmp line
  value="$(clean_value "$3")"
  tmp="$(mktemp "${file}.XXXXXX")"
  if [[ -f "$file" ]]; then
    while IFS= read -r line; do
      [[ "${line%%=*}" == "$key" ]] || printf '%s\n' "$line"
    done <"$file" >"$tmp"
  fi
  printf '%s=%s\n' "$key" "$value" >>"$tmp"
  mv "$tmp" "$file"
}

init_state() {
  [[ -n "$TOOL" ]] || die "--tool is required"
  TOOL_SLUG="$(slugify "$TOOL")"
  [[ "$WORKER_ID" =~ ^[a-z0-9][a-z0-9.-]*$ ]] || die "invalid --worker-id '$WORKER_ID' (use [a-z0-9.-])"
  STATE_ROOT="$(git -C "$REPO_ROOT" rev-parse --path-format=absolute --git-common-dir)/cache-intel"
  WORK_DIR="$STATE_ROOT/work/${TOOL_SLUG}-${WORKER_ID}"
  mkdir -p "$WORK_DIR/evidence" "$WORK_DIR/remote" "$WORK_DIR/out"
  STATE_FILE="$WORK_DIR/state.env"
  LEDGER="$WORK_DIR/ledger.tsv"
  METRICS="$WORK_DIR/metrics.env"
  RESEARCH="$WORK_DIR/research.md"
  DESIGN="$WORK_DIR/design.md"
  RUNS="$WORK_DIR/remote/runs.tsv"
  HC_ENV_FILE="$WORK_DIR/out/harness-core.env"
  SUMMARY_FILE="$WORK_DIR/out/summary.env"
  if [[ ! -f "$STATE_FILE" ]]; then
    kv_set "$STATE_FILE" TOOL "$TOOL"
    kv_set "$STATE_FILE" WORKER_ID "$WORKER_ID"
    kv_set "$STATE_FILE" BASE_BRANCH "${BASE_BRANCH:-$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)}"
    kv_set "$STATE_FILE" STARTING_SHA "$(git -C "$REPO_ROOT" rev-parse HEAD)"
    kv_set "$STATE_FILE" CREATED_AT "$(date -u +%FT%TZ)"
  fi
  [[ -n "$BASE_BRANCH" ]] || BASE_BRANCH="$(kv_get "$STATE_FILE" BASE_BRANCH)"
  [[ -n "$IMAGE_TAG" ]] || IMAGE_TAG="$(kv_get "$STATE_FILE" CANDIDATE_IMAGE)"
  [[ -n "$HC_DIR" ]] || HC_DIR="$STATE_ROOT/harness-core"
  load_fields
}

record() {
  local gate="$1" result="$2" evidence
  evidence="$(clean_value "$3")"
  evidence="${evidence//$WORK_DIR/<work>}"
  evidence="${evidence//$REPO_ROOT\//}"
  evidence="${evidence% }"
  printf '%s\t%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$gate" "$result" "$evidence" >>"$LEDGER"
  log "${gate}: ${result} — ${evidence}"
}

gate_result() {
  [[ -f "$LEDGER" ]] || return 0
  awk -F'\t' -v g="$1" '$2 == g { r = $3 } END { if (r != "") print r }' "$LEDGER"
}

require_gate() {
  local gate="$1" r
  r="$(gate_result "$gate")"
  [[ "$r" == "PASS" || "$r" == "NOT_APPLICABLE" ]] || die "gate '$gate' has not passed yet (last: ${r:-not run}); run it first"
}

metric() { kv_set "$METRICS" "$1" "$2"; }

load_fields() {
  F=()
  [[ -f "$RESEARCH" ]] || return 0
  local k v
  while IFS=$'\t' read -r k v; do
    F["$k"]="$v"
  done < <(python3 "$HELPERS" fields "$RESEARCH")
}

# field <name>: command-line override, else research.md value; "none" means empty.
field() {
  local v="${OPT[$1]:-${F[$1]:-}}"
  [[ "$v" == "none" ]] && v=""
  printf '%s' "$v"
}

cache_env_key() { local e; e="$(field cache_env)"; [[ -n "$e" ]] && printf '%s' "${e%%=*}"; }
cache_env_dir() { local e; e="$(field cache_env)"; [[ -n "$e" ]] && printf '%s' "${e#*=}"; }
tool_id() { local t; t="$(field tool_id)"; printf '%s' "${t:-$TOOL_SLUG}"; }
test_names() { printf '%s\n%s\n' "${TOOL_SLUG//-/}" "$(tool_id | tr -d '-')" | sort -u | paste -sd'|' -; }
test_pattern() { printf '(?i)(%s)' "$(test_names)"; }

now() { printf '%s' "$EPOCHREALTIME"; }
elapsed() { awk -v a="$1" -v b="$2" 'BEGIN { printf "%.2f", b - a }'; }

# ------------------------------------------------------------------------ help / status

cmd_help() {
  usage
  cat <<EOF

Skill: ${SKILL_DIR}/SKILL.md (read COMMANDS.md for each command's contract).
State: <git-common-dir>/cache-intel/work/<tool>-<worker>/ (research.md, design.md, ledger.tsv, metrics.env,
       remote/, out/). It lives outside the working tree, so it never enters the product diff.
Images: the agent container has no Docker daemon. The orchestration pipeline builds and pushes the
       candidate with BuildAndPushDockerRegistry and passes it to the remote-validate phase.
Logs:   scripts/fetch-step-log.sh '<download_url from harness_get execution_log>'.
EOF
}

next_step() {
  local g
  for g in research design create test-local test-compatibility test-public build finalize harness-core; do
    if [[ -z "$(gate_result "$g")" || "$(gate_result "$g")" == "FAIL" || "$(gate_result "$g")" == "PENDING" ]]; then
      case "$g" in
        test-*) echo "test --${g#test-}" ;;
        *) echo "$g" ;;
      esac
      return
    fi
  done
  echo "remote-setup (remote-validate phase) or report"
}

cmd_status() {
  init_state
  log "tool=${TOOL} id=$(tool_id) worker=${WORKER_ID} base=${BASE_BRANCH} work=${WORK_DIR}"
  if grep -q "tool:[[:space:]]*\"$(tool_id)\"" "$AUTODETECT_DIR/auto_detect_util.go" 2>/dev/null; then
    log "mapping: '$(tool_id)' is present in buildToolInfoMapping"
  else
    log "mapping: '$(tool_id)' is not in buildToolInfoMapping"
  fi
  printf '%-20s %-15s %s\n' GATE RESULT EVIDENCE
  local g
  for g in research design create test-local test-compatibility test-lifecycle test-public build finalize \
    harness-core remote-cold remote-warm remote-invalidate product-seed product-warm; do
    local row
    row="$(awk -F'\t' -v g="$g" '$2 == g { r = $3 "\t" $4 } END { print r }' "$LEDGER" 2>/dev/null || true)"
    printf '%-20s %-15s %s\n' "$g" "${row%%$'\t'*}" "$( [[ "$row" == *$'\t'* ]] && echo "${row#*$'\t'}" | cut -c1-90)"
  done
  log "next: $(next_step)"
}

# ------------------------------------------------------------------------ research / design

existing_support() {
  local out="" id
  id="$(tool_id)"
  if grep -q "tool:[[:space:]]*\"${id}\"" "$AUTODETECT_DIR/auto_detect_util.go" 2>/dev/null; then
    out+="- buildToolInfoMapping already has tool \`${id}\`: extend existing support (characterize current behavior first)."$'\n'
  else
    out+="- buildToolInfoMapping has no \`${id}\` entry: new tool within the engine."$'\n'
  fi
  local prep="$AUTODETECT_DIR/prepare_${TOOL_SLUG//-/_}.go"
  [[ -f "$prep" ]] && out+="- Preparer exists: \`${prep#"$REPO_ROOT"/}\`."$'\n'
  out+="- Detected tools today: $(grep -o 'tool:[[:space:]]*"[^"]*"' "$AUTODETECT_DIR/auto_detect_util.go" | sed 's/.*"\(.*\)"/\1/' | sort -u | tr '\n' ' ')"$'\n'
  local constants="$HC_DIR/879-pipeline-ci-commons/src/main/java/io/harness/ci/commonconstants/ContainerExecutionConstants.java"
  if [[ -f "$constants" ]]; then
    out+="- harness-core already injects (Cache Intelligence on): $(grep -o '_CACHE_ENV_NAME = "[^"]*"' "$constants" | sed 's/.*"\(.*\)"/\1/' | tr '\n' ' ')"$'\n'
  else
    out+="- harness-core already injects (Cache Intelligence on, as of develop 2026-10): GOCACHE GRADLE_USER_HOME npm_config_cache PIP_CACHE_DIR PIPENV_CACHE_DIR POETRY_CACHE_DIR UV_CACHE_DIR CP_CACHE_DIR"$'\n'
  fi
  printf '%s' "$out"
}

render_doc() {
  local tmpl="$1" out="$2" body
  body="$(cat "$tmpl")"
  body="${body//\{\{TOOL\}\}/$TOOL}"
  body="${body//\{\{TOOL_SLUG\}\}/$TOOL_SLUG}"
  body="${body//\{\{EXISTING_SUPPORT\}\}/$(existing_support)}"
  printf '%s\n' "$body" >"$out"
}

check_doc() {
  local kind="$1" file="$2" gate="$3" problems
  if problems="$(python3 "$HELPERS" check "$kind" "$file")"; then
    return 0
  fi
  record "$gate" PENDING "$(echo "$problems" | head -3 | tr '\n' ' ')"
  log "Complete ${file}:"
  printf '%s\n' "$problems" >&2
  exit 2
}

cmd_research() {
  init_state
  if [[ ! -f "$RESEARCH" ]]; then
    render_doc "$SCRIPT_DIR/research.tmpl.md" "$RESEARCH"
    record research PENDING "scaffold written to ${RESEARCH}"
    log "Fill every field and section of ${RESEARCH} (official docs + tool commands), then re-run research."
    exit 2
  fi
  check_doc research "$RESEARCH" research
  load_fields
  record research PASS "detect=$(field detect_files); paths=$(field cache_paths); cache_env=$(field cache_env); fixture=$(field fixture_repo)@$(field fixture_ref)"
}

cmd_design() {
  init_state
  require_gate research
  if [[ ! -f "$DESIGN" ]]; then
    render_doc "$SCRIPT_DIR/design.tmpl.md" "$DESIGN"
    record design PENDING "scaffold written to ${DESIGN}"
    log "Fill ${DESIGN} using ENGINE.md, then re-run design."
    exit 2
  fi
  check_doc design "$DESIGN" design
  record design PASS "design record complete (${DESIGN})"
}

# ------------------------------------------------------------------------ create / test / build

tests_passed() { grep -c -- '--- PASS' "$1" || true; }

cmd_create() {
  init_state
  require_gate design
  local id
  id="$(tool_id)"
  grep -q "tool:[[:space:]]*\"${id}\"" "$AUTODETECT_DIR/auto_detect_util.go" || die "create: tool \"${id}\" is not registered in buildToolInfoMapping"
  ls "$AUTODETECT_DIR"/prepare_*_test.go >/dev/null
  local logf="$WORK_DIR/evidence/create.log" n
  (cd "$REPO_ROOT" && go test -count=1 -v -run "$(test_pattern)" ./internal/plugin/autodetect -timeout 10m) >"$logf" 2>&1 \
    || { tail -40 "$logf" >&2; record create FAIL "focused tests failed ($logf)"; exit 1; }
  n="$(tests_passed "$logf")"
  [[ "$n" -gt 0 ]] || { record create FAIL "no test matched '$(test_pattern)'; name the preparer tests after the tool"; exit 1; }
  record create PASS "mapping has ${id}; ${n} focused tests passed (go test -run '$(test_pattern)')"
}

changed_go_files() {
  git -C "$REPO_ROOT" diff --name-only --diff-filter=ACM "$(kv_get "$STATE_FILE" STARTING_SHA)" -- '*.go'
  git -C "$REPO_ROOT" ls-files --others --exclude-standard -- '*.go'
}

test_local() {
  local logf="$WORK_DIR/evidence/test-local.log" unformatted
  {
    (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect ./internal/plugin ./cache/... -timeout 20m)
    (cd "$REPO_ROOT" && go vet ./...)
    (cd "$REPO_ROOT" && go build ./...)
  } >"$logf" 2>&1 || { tail -40 "$logf" >&2; record test-local FAIL "go test/vet/build failed ($logf)"; exit 1; }
  unformatted="$(cd "$REPO_ROOT" && changed_go_files | sort -u | xargs -r gofmt -l)"
  [[ -z "$unformatted" ]] || { record test-local FAIL "gofmt needed: ${unformatted}"; exit 1; }
  record test-local PASS "go test autodetect+plugin+cache, go vet ./..., go build ./..., gofmt clean"
}

test_compatibility() {
  local logf="$WORK_DIR/evidence/test-compatibility.log" n names
  names="$(test_names)"
  (cd "$REPO_ROOT" && go test -count=1 -v -run "(?i)(Scenario.*(${names})|(${names}).*Scenario)" ./internal/plugin/autodetect -timeout 10m) >"$logf" 2>&1 \
    || { tail -40 "$logf" >&2; record test-compatibility FAIL "scenario tests failed ($logf)"; exit 1; }
  n="$(tests_passed "$logf")"
  [[ "$n" -gt 0 ]] || { record test-compatibility FAIL "no scenario test matched; add Test<Tool>Scenarios covering design.md's scenario matrix"; exit 1; }
  (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect -timeout 15m) >>"$logf" 2>&1 \
    || { tail -40 "$logf" >&2; record test-compatibility FAIL "legacy autodetect tests failed ($logf)"; exit 1; }
  record test-compatibility PASS "${n} ${TOOL} scenario tests + full autodetect suite (legacy keys/paths)"
}

plugin_bin() {
  local bin="$WORK_DIR/bin/drone-cache"
  mkdir -p "$WORK_DIR/bin"
  (cd "$REPO_ROOT" && go build -o "$bin" .)
  printf '%s' "$bin"
}

plugin_env() {
  # Explicit allowlist: PLUGIN_DEBUG would print the environment, so never inherit the host env.
  local cache_root="$1" home="$2"
  printf '%s\n' "PATH=/usr/bin:/bin" "HOME=$home" "PLUGIN_AUTO_CACHE=true" "PLUGIN_BACKEND=filesystem" \
    "PLUGIN_FILESYSTEM_CACHE_ROOT=$cache_root" "PLUGIN_ACCOUNT_ID=cache-intel" "PLUGIN_OVERRIDE=true" \
    "PLUGIN_ARCHIVE_FORMAT=tar" "PLUGIN_LOG_LEVEL=debug" "PLUGIN_DEBUG=false" "DRONE_REPO_NAMESPACE=local" \
    "DRONE_REPO_NAME=${TOOL_SLUG}-fixture" "DRONE_COMMIT_BRANCH=main" "DRONE_COMMIT_SHA=cache-intel"
}

run_plugin() {
  local mode="$1" ws="$2" logf="$3" cache_root="$4" home="$5" extra=()
  local key dir
  key="$(cache_env_key)"; dir="$(cache_env_dir)"
  [[ -n "$key" ]] && extra+=("${key}=${ws}/${dir}")
  local -a env_list
  mapfile -t env_list < <(plugin_env "$cache_root" "$home")
  if [[ "$mode" == restore ]]; then
    (cd "$ws" && env -i "${env_list[@]}" "${extra[@]}" PLUGIN_RESTORE=true PLUGIN_REBUILD=false "$PLUGIN" >"$logf" 2>&1) || true
  else
    (cd "$ws" && env -i "${env_list[@]}" "${extra[@]}" PLUGIN_RESTORE=false PLUGIN_REBUILD=true "$PLUGIN" >"$logf" 2>&1) \
      || { tail -30 "$logf" >&2; die "save failed ($logf)"; }
  fi
}

restored() { [[ -e "$1" && -n "$(ls -A "$1" 2>/dev/null)" ]]; }

docker_usable() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }

run_tool() {
  # run_tool <ws> <subdir> <logf> <cmd>: runs a tool command with the research cache_env wired the way
  # harness-core injects it (an absolute path under the workspace). The host run keeps the real HOME
  # because toolchain managers (rustup, pyenv, nvm) live there.
  local ws="$1" subdir="$2" logf="$3" cmd="$4" key dir
  key="$(cache_env_key)"; dir="$(cache_env_dir)"
  if [[ "$ACTIVE_RUNNER" == docker ]]; then
    local -a envs=(-e HOME=/tmp)
    [[ -n "$key" ]] && envs+=(-e "${key}=/harness/${dir}")
    docker run --rm --user "$(id -u):$(id -g)" -v "$ws:/harness" -w "/harness/${subdir}" "${envs[@]}" \
      "$(field install_image)" sh -c "$cmd" >"$logf" 2>&1
  else
    local -a env_args=()
    local v
    IFS=',' read -r -a unset_list <<<"$(field unset_env)"
    for v in "${unset_list[@]}"; do [[ -n "$v" ]] && env_args+=(-u "$v"); done
    [[ -n "$key" ]] && env_args+=(-u "$key" "${key}=${ws}/${dir}")
    (cd "$ws/$subdir" && env "${env_args[@]}" sh -c "$cmd" >"$logf" 2>&1)
  fi
}

archive_scan() {
  # Lists every saved archive and fails when an excluded path segment was archived.
  local cache_root="$1" listing="$WORK_DIR/evidence/public-archive-listing.txt" seg hits=""
  : >"$listing"
  while IFS= read -r f; do
    echo "== ${f#"$cache_root"/}" >>"$listing"
    tar -tf "$f" >>"$listing" 2>/dev/null || echo "(not a tar archive)" >>"$listing"
  done < <(find "$cache_root" -type f)
  IFS=',' read -r -a excluded <<<"$(field excluded_paths)"
  for seg in "${excluded[@]}"; do
    seg="$(echo "$seg" | xargs)"
    [[ -z "$seg" ]] && continue
    if grep -v '^==' "$listing" | grep -Eq "(^|/)${seg//./\\.}(/|$)"; then hits+="${seg} "; fi
  done
  if [[ -n "$hits" ]]; then
    record test-public FAIL "archive contains excluded paths: ${hits}(see ${listing})"
    exit 1
  fi
  printf 'clean (%s entries, no excluded segment)' "$(grep -vc '^==' "$listing" || true)"
}

plugin_key() { grep -Eo 'remote=cache-intel/[0-9a-f]{32}' "$1" | head -1 | cut -d/ -f2 || true; }

test_public() {
  local repo ref install offline warm inval first
  repo="$(field fixture_repo)"; ref="$(field fixture_ref)"; install="$(field install_cmd)"
  offline="$(field offline_cmd)"; warm="$(field warm_check)"; inval="$(field invalidate_file)"
  [[ -n "$repo" && -n "$ref" && -n "$install" && -n "$warm" && -n "$inval" ]] \
    || die "research fields fixture_repo, fixture_ref, install_cmd, warm_check and invalidate_file are required"

  ACTIVE_RUNNER="$RUNNER"
  if [[ "$ACTIVE_RUNNER" == auto ]]; then
    first="${install%% *}"
    if command -v "$first" >/dev/null 2>&1; then ACTIVE_RUNNER=host
    elif [[ -n "$(field install_image)" ]] && docker_usable; then ACTIVE_RUNNER=docker
    else
      record test-public FAIL "'${first}' is not on PATH and Docker is unavailable; install the tool in user space (SKILL.md section 6) or use --runner docker"
      exit 1
    fi
  fi
  [[ "$ACTIVE_RUNNER" == docker ]] && { docker_usable || die "--runner docker but Docker is unavailable"; }

  local base="$WORK_DIR/public" ws cache_root home ev="$WORK_DIR/evidence" sub
  rm -rf "$base"; mkdir -p "$base/home" "$base/backend"
  ws="$base/ws"; cache_root="$base/backend"; home="$base/home"
  sub="$(field fixture_subdir)"; sub="${sub:-.}"
  log "public lifecycle: ${repo}@${ref} runner=${ACTIVE_RUNNER}"
  git init -q "$ws"
  git -C "$ws" remote add origin "$repo"
  git -C "$ws" fetch -q --depth 1 origin "$ref" || die "cannot fetch ${repo} ${ref}"
  git -C "$ws" checkout -q FETCH_HEAD
  metric LOCAL_FIXTURE_SHA "$(git -C "$ws" rev-parse HEAD)"
  metric LOCAL_RUNNER "$ACTIVE_RUNNER$( [[ "$ACTIVE_RUNNER" == docker ]] && echo " ($(field install_image))")"
  PLUGIN="$(plugin_bin)"

  local t0 t1
  [[ "$ACTIVE_RUNNER" == docker ]] && { docker pull -q "$(field install_image)" >/dev/null || die "cannot pull $(field install_image)"; }
  run_tool "$ws" "$sub" "$ev/public-version.log" "${install%% *} --version" || true
  metric LOCAL_TOOL_VERSION "$(tail -1 "$ev/public-version.log" 2>/dev/null)"

  run_plugin restore "$ws" "$ev/public-1-cold-restore.log" "$cache_root" "$home"
  restored "$ws/$warm" && { record test-public FAIL "cold restore unexpectedly populated ${warm}"; exit 1; }

  t0="$(now)"
  run_tool "$ws" "$sub" "$ev/public-2-install-cold.log" "$install" \
    || { tail -30 "$ev/public-2-install-cold.log" >&2; record test-public FAIL "cold install failed ($ev/public-2-install-cold.log)"; exit 1; }
  t1="$(now)"; metric LOCAL_COLD_INSTALL_S "$(elapsed "$t0" "$t1")"
  restored "$ws/$warm" || { record test-public FAIL "install did not populate ${warm}; check cache_env/strategy"; exit 1; }

  t0="$(now)"; run_plugin save "$ws" "$ev/public-3-save.log" "$cache_root" "$home"; t1="$(now)"
  metric LOCAL_SAVE_S "$(elapsed "$t0" "$t1")"
  [[ -n "$(find "$cache_root" -type f -print -quit)" ]] || { record test-public FAIL "save produced no archive"; exit 1; }
  metric LOCAL_ARCHIVE_KB "$(du -sk "$cache_root" | cut -f1)"
  metric LOCAL_ARCHIVE_FILES "$(find "$cache_root" -type f | wc -l | tr -d ' ')"
  local scan
  scan="$(archive_scan "$cache_root")" || exit 1
  metric LOCAL_ARCHIVE_CHECK "$scan"
  metric LOCAL_KEY "$(plugin_key "$ev/public-3-save.log")"

  git -C "$ws" clean -ffdxq
  restored "$ws/$warm" && die "git clean did not remove ${warm}; it must be inside the workspace"

  t0="$(now)"; run_plugin restore "$ws" "$ev/public-5-warm-restore.log" "$cache_root" "$home"; t1="$(now)"
  metric LOCAL_WARM_RESTORE_S "$(elapsed "$t0" "$t1")"
  restored "$ws/$warm" || { tail -20 "$ev/public-5-warm-restore.log" >&2; record test-public FAIL "warm restore did not populate ${warm}"; exit 1; }
  metric LOCAL_WARM_KB "$(du -sk "$ws/$warm" | cut -f1)"

  if [[ -n "$offline" ]]; then
    t0="$(now)"
    run_tool "$ws" "$sub" "$ev/public-6-offline.log" "$offline" \
      || { tail -30 "$ev/public-6-offline.log" >&2; record test-public FAIL "offline check failed: the restored cache is incomplete"; exit 1; }
    t1="$(now)"; metric LOCAL_OFFLINE_S "$(elapsed "$t0" "$t1")"
  fi
  t0="$(now)"
  run_tool "$ws" "$sub" "$ev/public-7-install-warm.log" "$install" || { record test-public FAIL "warm install failed"; exit 1; }
  t1="$(now)"; metric LOCAL_WARM_INSTALL_S "$(elapsed "$t0" "$t1")"

  git -C "$ws" clean -ffdxq
  printf '\n' >>"$ws/$inval"
  run_plugin restore "$ws" "$ev/public-8-invalidate-restore.log" "$cache_root" "$home"
  restored "$ws/$warm" && { record test-public FAIL "restore hit after ${inval} changed: key does not cover it"; exit 1; }
  metric LOCAL_INVALIDATED_KEY "$(plugin_key "$ev/public-8-invalidate-restore.log")"
  [[ "$(kv_get "$METRICS" LOCAL_INVALIDATED_KEY)" != "$(kv_get "$METRICS" LOCAL_KEY)" ]] \
    || { record test-public FAIL "key did not change after ${inval} changed"; exit 1; }
  git -C "$ws" checkout -q -- "$inval"

  record test-public PASS "$(kv_get "$METRICS" LOCAL_RUNNER): cold $(kv_get "$METRICS" LOCAL_COLD_INSTALL_S)s, warm $(kv_get "$METRICS" LOCAL_WARM_INSTALL_S)s$( [[ -n "$offline" ]] && echo ", offline OK"), archive $(kv_get "$METRICS" LOCAL_ARCHIVE_KB)KB, invalidation missed"
}

load_profile() {
  local line
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^# || -z "${line// /}" ]] && continue
    [[ "${line%%|*}" == "$TOOL_SLUG" ]] && { echo "$line"; return 0; }
  done <"$PROFILES_FILE"
  return 1
}

test_lifecycle() {
  # In-repo fixtures from tool-profiles.env: cold restore, install, save, wipe, warm restore. Runs on a
  # copy so the repository stays clean; cache_env from research is wired automatically.
  local profile
  if ! profile="$(load_profile)"; then
    record test-lifecycle SKIPPED "no in-repo fixture profile in tool-profiles.env (public lifecycle covers the real install)"
    return 0
  fi
  local fixtures install clean unset_profile first
  IFS='|' read -r _ fixtures install clean unset_profile _ <<<"$profile"
  OPT[unset_env]="$(field unset_env)${unset_profile:+,${unset_profile}}"
  first="$(awk '{ for (i = 1; i <= NF; i++) if ($i !~ /=/) { print $i; exit } }' <<<"$install")"
  command -v "$first" >/dev/null 2>&1 || { record test-lifecycle SKIPPED "${first} not on PATH"; return 0; }
  ACTIVE_RUNNER=host
  PLUGIN="$(plugin_bin)"
  local f base ws cache_root ev="$WORK_DIR/evidence" check p ok=""
  IFS=',' read -r -a fixture_list <<<"$fixtures"
  IFS=',' read -r -a clean_paths <<<"$clean"
  for f in "${fixture_list[@]}"; do
    [[ -d "$REPO_ROOT/$f" ]] || { log "skip ${f}: missing"; continue; }
    base="$WORK_DIR/lifecycle/$(basename "$f")"
    rm -rf "$base"; mkdir -p "$base/home" "$base/backend"
    ws="$base/ws"; cache_root="$base/backend"
    cp -R "$REPO_ROOT/$f" "$ws"
    run_plugin restore "$ws" "$ev/lifecycle-cold-restore.log" "$cache_root" "$base/home"
    run_tool "$ws" . "$ev/lifecycle-install.log" "$install" \
      || { tail -20 "$ev/lifecycle-install.log" >&2; record test-lifecycle FAIL "install failed for ${f}"; exit 1; }
    run_plugin save "$ws" "$ev/lifecycle-save.log" "$cache_root" "$base/home"
    [[ -n "$(find "$cache_root" -type f -print -quit)" ]] || { record test-lifecycle FAIL "save produced no archive for ${f}"; exit 1; }
    for p in "${clean_paths[@]}"; do [[ -n "$p" ]] && rm -rf "${ws:?}/$p"; done
    run_plugin restore "$ws" "$ev/lifecycle-warm-restore.log" "$cache_root" "$base/home"
    check="$(field warm_check)"; check="${check:-${clean_paths[0]}}"
    restored "$ws/$check" || { record test-lifecycle FAIL "warm restore did not populate ${check} for ${f}"; exit 1; }
    ok+="${f} "
  done
  [[ -n "$ok" ]] || { record test-lifecycle SKIPPED "no fixture directory from tool-profiles.env exists"; return 0; }
  record test-lifecycle PASS "in-repo fixtures restored after wipe: ${ok}"
}

cmd_test() {
  init_state
  case "$TEST_MODE" in
    local) test_local ;;
    compatibility) test_compatibility ;;
    lifecycle) test_lifecycle ;;
    public) require_gate research; test_public ;;
    all) test_local; test_compatibility; test_lifecycle; test_public ;;
    *) die "unknown test mode: $TEST_MODE" ;;
  esac
}

cmd_build() {
  init_state
  local logf="$WORK_DIR/evidence/build.log"
  {
    mkdir -p "$REPO_ROOT/release/linux/amd64" "$REPO_ROOT/release/linux/arm64" "$REPO_ROOT/release/windows/amd64"
    (cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/drone-cache .)
    (cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o release/linux/arm64/drone-cache .)
    (cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o release/windows/amd64/drone-cache.exe .)
  } >"$logf" 2>&1 || { tail -30 "$logf" >&2; record build FAIL "cross-compile failed ($logf)"; exit 1; }
  local sum
  sum="$( (sha256sum "$REPO_ROOT/release/linux/amd64/drone-cache" 2>/dev/null || shasum -a 256 "$REPO_ROOT/release/linux/amd64/drone-cache") | cut -c1-16)"
  if $PUBLISH_IMAGE; then
    "$SCRIPT_DIR/docker-build-push.sh" --tool "$TOOL_SLUG" ${IMAGE_TAG:+--tag "$IMAGE_TAG"} || { record build BLOCKED "local docker publish failed"; exit 3; }
  fi
  record build PASS "linux/amd64 (sha256 ${sum}…), linux/arm64, windows/amd64; image is built by the pipeline"
}

cmd_finalize() {
  init_state
  local missing=""
  grep -qi -- "$TOOL" "$REPO_ROOT/README.md" || missing+="README.md "
  grep -qi -- "$TOOL" "$REPO_ROOT/CHANGELOG.md" 2>/dev/null || missing+="CHANGELOG.md "
  [[ -z "$missing" ]] || { record finalize FAIL "no ${TOOL} entry in ${missing}"; exit 1; }
  record finalize PASS "README and CHANGELOG mention ${TOOL}"
}

# ------------------------------------------------------------------------ harness-core

cmd_harness_core() {
  init_state
  require_gate research
  local key dir const
  key="$(cache_env_key)"; dir="$(cache_env_dir)"
  if [[ -z "$key" ]]; then
    printf 'HC_REQUIRED=false\nHC_STATUS=NOT_APPLICABLE\n' >"$HC_ENV_FILE"
    record harness-core NOT_APPLICABLE "research cache_env is none: the plugin finds the cache from the workspace alone"
    return 0
  fi
  if [[ ! -d "$HC_DIR/.git" ]]; then
    printf 'HC_REQUIRED=true\nHC_STATUS=BLOCKED\nHC_ENV=%s\nHC_SUFFIX=%s\n' "$key" "$dir" >"$HC_ENV_FILE"
    record harness-core BLOCKED "no harness-core checkout at ${HC_DIR}; owner: orchestration pipeline (prepare_harness_core step, secret harness0_code_pat)"
    return 0
  fi
  const="$(echo "$TOOL_SLUG" | tr '[:lower:]-' '[:upper:]_')"
  local branch="feature/${JIRA_ID:+${JIRA_ID}-}cache-intel-${TOOL_SLUG}-env-${WORKER_ID}" title rc=0 out
  if [[ -n "$JIRA_ID" ]]; then
    title="feat: [${JIRA_ID}]: inject ${key} for ${TOOL} Cache Intelligence"
  else
    title="feat: inject ${key} for ${TOOL} Cache Intelligence"
  fi
  if [[ "$(git -C "$HC_DIR" rev-parse --abbrev-ref HEAD)" == "$branch" \
    && "$(kv_get "$HC_ENV_FILE" HC_COMMIT)" == "$(git -C "$HC_DIR" rev-parse HEAD)" ]]; then
    record harness-core PASS "already committed on ${branch} ($(kv_get "$HC_ENV_FILE" HC_COMMIT | cut -c1-12))"
    return 0
  fi
  git -C "$HC_DIR" diff --quiet || die "harness-core checkout ${HC_DIR} has uncommitted changes"
  git -C "$HC_DIR" checkout -q -b "$branch" || die "branch ${branch} already exists in ${HC_DIR}; delete it or use another --worker-id"
  local -a gen_args=(--repo "$HC_DIR" --env "$key" --suffix "$dir" --const "$const" --comment "$(field cache_env_reason)")
  $HC_WINDOWS || gen_args+=(--no-windows)
  out="$(python3 "$SCRIPT_DIR/harness-core-env.py" "${gen_args[@]}" 2>&1)" || rc=$?
  printf '%s\n' "$out" >"$WORK_DIR/evidence/harness-core.log"
  if [[ $rc -eq 3 ]]; then
    printf 'HC_REQUIRED=false\nHC_STATUS=ALREADY_INJECTED\nHC_ENV=%s\nHC_SUFFIX=%s\n' "$key" "$dir" >"$HC_ENV_FILE"
    record harness-core NOT_APPLICABLE "harness-core already injects ${key}"
    return 0
  fi
  [[ $rc -eq 0 ]] || { printf '%s\n' "$out" >&2; record harness-core FAIL "generator failed: $(echo "$out" | tail -1)"; exit 1; }
  git -C "$HC_DIR" diff --stat >>"$WORK_DIR/evidence/harness-core.log"
  git -C "$HC_DIR" -c user.name="Harness Cache Intelligence" -c user.email="cache-intel@harness.io" \
    commit -q -am "$title" -m "Cache Intelligence worker ${WORKER_ID}: ${TOOL} caches $(field cache_paths), which only lands in the workspace when ${key} is set."
  git -C "$HC_DIR" format-patch -1 --stdout >"$WORK_DIR/out/harness-core.patch"
  {
    echo "HC_REQUIRED=true"
    echo "HC_STATUS=COMMITTED"
    echo "HC_BRANCH=${branch}"
    echo "HC_TITLE=${title}"
    echo "HC_ENV=${key}"
    echo "HC_SUFFIX=${dir}"
    echo "HC_COMMIT=$(git -C "$HC_DIR" rev-parse HEAD)"
  } >"$HC_ENV_FILE"
  record harness-core PASS "${key}=<workDir>/${dir} added to VM + K8s and 3 test classes on ${branch} ($(git -C "$HC_DIR" diff --shortstat HEAD~1 | xargs))"
}

# ------------------------------------------------------------------------ remote

yaml_dq_escape() { local s="$1"; s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; printf '%s' "$s"; }

remote_pipeline_id() {
  local raw="ci_intel_${TOOL_SLUG}_${WORKER_ID}${1:-}"
  echo "$raw" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_\n' '_' | cut -c1-120
}

stage_variables() {
  local key dir
  key="$(cache_env_key)"; dir="$(cache_env_dir)"
  if [[ -z "$key" ]]; then printf ' []'; return; fi
  printf '\n          - name: %s\n            type: String\n            value: /harness/%s' "$key" "$dir"
}

render_template() {
  # render_template <template> <out> <pipeline-id> <harness_code:true|false>
  local tmpl="$1" out="$2" id="$3" hc="$4" body subdir key
  subdir="$(field fixture_subdir)"; subdir="${subdir:-.}"
  key="$(cache_env_key)"
  local source=github fixture_repo
  fixture_repo="$(field fixture_repo)"
  if [[ "$hc" == true ]]; then source=harness_code; fixture_repo="$(field harness_code_repo)"; fi
  body="$(awk -v hc="$hc" '
    /^#@if harness_code/ { inblk = 1; keep = (hc == "true"); next }
    /^#@else/ { keep = !keep; next }
    /^#@end/ { inblk = 0; next }
    { if (!inblk || keep) print }' "$tmpl")"
  body="${body//__PIPELINE_ID__/$id}"
  body="${body//__ORG__/$HARNESS_ORG}"
  body="${body//__PROJECT__/$HARNESS_PROJECT}"
  body="${body//__TOOL__/$TOOL_SLUG}"
  body="${body//__WORKER__/$WORKER_ID}"
  body="${body//__IMAGE__/$IMAGE_TAG}"
  body="${body//__DOCKER_CONNECTOR__/$DOCKER_CONNECTOR}"
  body="${body//__HC_REPO__/$(field harness_code_repo)}"
  body="${body//__HC_BRANCH__/${OPT[harness_code_branch]:-main}}"
  body="${body//__FIXTURE_SOURCE__/$source}"
  body="${body//__FIXTURE_REPO__/$(yaml_dq_escape "$fixture_repo")}"
  body="${body//__FIXTURE_REF__/$(yaml_dq_escape "$(field fixture_ref)")}"
  body="${body//__FIXTURE_SUBDIR__/$subdir}"
  body="${body//__INSTALL_IMAGE__/$(field install_image)}"
  body="${body//__INSTALL_CMD__/$(yaml_dq_escape "$(field install_cmd)")}"
  body="${body//__OFFLINE_CMD__/$(yaml_dq_escape "$(field offline_cmd)")}"
  body="${body//__WARM_CHECK__/$(yaml_dq_escape "$(field warm_check)")}"
  body="${body//__INVALIDATE_FILE__/$(yaml_dq_escape "$(field invalidate_file)")}"
  body="${body//__ENV_NAMES__/${key:-NO_CACHE_ENV}}"
  body="${body//__STAGE_VARIABLES__/$(stage_variables)}"
  if grep -q '__[A-Z_]*__' <<<"$body"; then die "unrendered placeholder in ${tmpl}: $(grep -o '__[A-Z_]*__' <<<"$body" | sort -u | tr '\n' ' ')"; fi
  printf '%s\n' "$body" >"$out"
}

cmd_remote_setup() {
  init_state
  require_gate research
  [[ -n "$IMAGE_TAG" ]] || die "--image <repo:tag> is required (the candidate pushed by the pipeline)"
  local f
  for f in fixture_repo fixture_ref install_image install_cmd warm_check invalidate_file; do
    [[ -n "$(field "$f")" ]] || die "research field ${f} is empty"
  done
  kv_set "$STATE_FILE" CANDIDATE_IMAGE "$IMAGE_TAG"
  local hc=false
  [[ -n "$(field harness_code_repo)" && "${OPT[fixture_source]:-}" == harness_code ]] && hc=true
  local id out
  id="$(remote_pipeline_id)"
  out="$WORK_DIR/remote/${id}.yaml"
  render_template "$SCRIPT_DIR/remote-pipeline.tmpl.yaml" "$out" "$id" "$hc"
  echo "PIPELINE_ID=${id}"
  echo "PIPELINE_YAML=${out}"
  local source evidence
  if $hc; then source="$(field harness_code_repo)"; else source="$(field fixture_repo)"; fi
  evidence="pipeline=${id}; image=${IMAGE_TAG}; fixture=${source}@$(field fixture_ref); cache_env=$(field cache_env)"
  if $PRODUCT; then
    [[ -n "$(field harness_code_repo)" ]] || die "--product needs research field harness_code_repo (create it with MCP and fixture-commit)"
    local pid pout
    pid="$(remote_pipeline_id _product)"
    pout="$WORK_DIR/remote/${pid}.yaml"
    render_template "$SCRIPT_DIR/remote-product-pipeline.tmpl.yaml" "$pout" "$pid" true
    echo "PRODUCT_PIPELINE_ID=${pid}"
    echo "PRODUCT_PIPELINE_YAML=${pout}"
    evidence+="; product=${pid}"
  fi
  cat <<EOF
ORG=${HARNESS_ORG}
PROJECT=${HARNESS_PROJECT}
Next, with Harness MCP (scope ${HARNESS_ORG}/${HARNESS_PROJECT}):
  1. harness_create resource_type=pipeline body=<YAML file contents>; if it exists, harness_update with the same body.
  2. Run in order, waiting for each to finish (harness_diagnose on the execution URL):
       inputs {scenario: "cold"} → {scenario: "warm"} → {scenario: "invalidate"}
  3. Read step logs: harness_get resource_type=execution_log params {execution_id, stage_id: validate,
     step_id: <step>, return_download_url: true} → scripts/fetch-step-log.sh '<download_url>' 200
  4. Record each: cache-intel.sh remote-test --run <scenario> --execution-url <url> --result PASS|FAIL \\
       --metrics install_seconds=<TIMING_install_seconds>,cache_kb=<CACHE_KB or RESTORED_KB>
EOF
  record remote-setup PASS "$evidence"
}

cmd_remote_test() {
  init_state
  case "$REMOTE_RUN" in
    cold|warm|invalidate|product-seed|product-warm) ;;
    *) die "--run must be cold, warm, invalidate, product-seed or product-warm" ;;
  esac
  case "$REMOTE_RESULT" in PASS|FAIL|BLOCKED) ;; *) die "--result must be PASS, FAIL or BLOCKED" ;; esac
  [[ "$REMOTE_RESULT" == BLOCKED || -n "$EXECUTION_URL" ]] || die "--execution-url is required"
  local metrics="${RUN_METRICS//,/;}"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$REMOTE_RUN" "$REMOTE_RESULT" "$EXECUTION_URL" "${IMAGE_TAG:-unknown}" "$metrics" >>"$RUNS"
  local gate="remote-${REMOTE_RUN}"
  [[ "$REMOTE_RUN" == product-* ]] && gate="$REMOTE_RUN"
  record "$gate" "$REMOTE_RESULT" "${EXECUTION_URL:-$REASON}${metrics:+; $metrics}"
  [[ "$REMOTE_RESULT" != FAIL ]] || exit 1
}

# ------------------------------------------------------------------------ validate / report

cmd_validate() {
  init_state
  local logf="$WORK_DIR/evidence/validate.log" failed="" missing="" blocked="" g r
  if ! (cd "$REPO_ROOT" && go test -count=1 ./internal/plugin/autodetect -timeout 15m) >"$logf" 2>&1; then
    failed+="autodetect-suite "
  fi
  for g in research design create test-local test-compatibility test-public build finalize harness-core; do
    r="$(gate_result "$g")"
    case "$r" in
      PASS|NOT_APPLICABLE|SKIPPED) printf 'PASS  %-20s %s\n' "$g" "$r" ;;
      BLOCKED) blocked+="$g "; printf 'BLOCK %-20s\n' "$g" ;;
      FAIL) failed+="$g "; printf 'FAIL  %-20s\n' "$g" ;;
      *) missing+="$g "; printf 'MISS  %-20s %s\n' "$g" "${r:-not run}" ;;
    esac
  done
  local remote_ok=true
  for g in remote-cold remote-warm remote-invalidate; do
    r="$(gate_result "$g")"
    printf '%-5s %-20s %s\n' "${r:-MISS}" "$g" ""
    [[ "$r" == FAIL ]] && failed+="$g "
    [[ "$r" == PASS ]] || remote_ok=false
  done
  for g in product-seed product-warm; do
    r="$(gate_result "$g")"; [[ -n "$r" ]] && printf '%-5s %-20s (optional)\n' "$r" "$g"
    [[ "$r" == FAIL ]] && failed+="$g "
  done
  local state
  if [[ -n "$failed" ]]; then state=FAILED
  elif [[ -n "$missing" ]]; then state=INCOMPLETE
  elif $LOCAL_ONLY || [[ -z "$ENV_NAME" ]]; then state=LOCAL_COMPLETE
  elif $remote_ok; then state=REMOTE_VALIDATED
  else state=REMOTE_BLOCKED
  fi
  {
    echo "TERMINAL_STATE=${state}"
    echo "FAILED_GATES=${failed}"
    echo "MISSING_GATES=${missing}"
    echo "BLOCKED_GATES=${blocked}"
  } >"$SUMMARY_FILE"
  kv_set "$STATE_FILE" TERMINAL_STATE "$state"
  log "Terminal state: ${state}${failed:+ (failed: $failed)}${missing:+ (missing: $missing)}${blocked:+ (blocked: $blocked)}"
  [[ "$state" != FAILED && "$state" != INCOMPLETE ]]
}

cmd_report() {
  init_state
  local branch
  branch="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
  [[ "$branch" != "HEAD" ]] || branch="feat/${TOOL}-caching-${WORKER_ID}"
  python3 "$HELPERS" report --work "$WORK_DIR" --repo "$REPO_ROOT" --tool "$TOOL" --worker "$WORKER_ID" \
    --branch "$branch" --repo-name "$REPO_NAME" --image "$IMAGE_TAG"
  record report PASS "REPORT.md and PR bodies regenerated"
}

cmd_waive() {
  init_state
  [[ -n "$GATE" && -n "$REASON" ]] || die "--gate and --reason are required"
  case "$REMOTE_RESULT" in BLOCKED|NOT_APPLICABLE|SKIPPED) ;; *) die "--result must be BLOCKED, NOT_APPLICABLE or SKIPPED" ;; esac
  record "$GATE" "$REMOTE_RESULT" "$REASON"
}

cmd_fixture_commit() {
  init_state
  [[ -n "$FROM_DIR" ]] || die "--from <dir> is required"
  local out="$WORK_DIR/remote/fixture-commit.json"
  python3 "$HELPERS" fixture-commit --from "$FROM_DIR" --out "$out" --files "$FIXTURE_FILES" \
    --title "Cache Intelligence fixture: $(field fixture_repo)@$(field fixture_ref)"
}

cmd_all() {
  cmd_research
  cmd_design
  cmd_create
  TEST_MODE=all cmd_test
  cmd_build
  cmd_finalize
  cmd_harness_core
  cmd_validate
  cmd_report
}

# ------------------------------------------------------------------------ main

parse_args() {
  [[ $# -ge 1 ]] || { usage; exit 2; }
  COMMAND="$1"; shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --tool) TOOL="${2:?}"; shift 2 ;;
      --worker-id) WORKER_ID="${2:?}"; shift 2 ;;
      --env) ENV_NAME="${2:?}"; shift 2 ;;
      --local) LOCAL_ONLY=true; shift ;;
      --publish) PUBLISH_IMAGE=true; shift ;;
      --image) IMAGE_TAG="${2:?}"; shift 2 ;;
      --branch) BASE_BRANCH="${2:?}"; shift 2 ;;
      --org) HARNESS_ORG="${2:?}"; shift 2 ;;
      --project) HARNESS_PROJECT="${2:?}"; shift 2 ;;
      --docker-connector) DOCKER_CONNECTOR="${2:?}"; shift 2 ;;
      --repo-name) REPO_NAME="${2:?}"; shift 2 ;;
      --runner) RUNNER="${2:?}"; shift 2 ;;
      --product) PRODUCT=true; shift ;;
      --fixture-source) OPT[fixture_source]="${2:?}"; shift 2 ;;
      --fixture-repo) OPT[fixture_repo]="${2:?}"; shift 2 ;;
      --fixture-ref) OPT[fixture_ref]="${2:?}"; shift 2 ;;
      --fixture-subdir) OPT[fixture_subdir]="${2:?}"; shift 2 ;;
      --install-image) OPT[install_image]="${2:?}"; shift 2 ;;
      --install-cmd) OPT[install_cmd]="${2:?}"; shift 2 ;;
      --offline-cmd) OPT[offline_cmd]="${2:?}"; shift 2 ;;
      --warm-check) OPT[warm_check]="${2:?}"; shift 2 ;;
      --invalidate-file) OPT[invalidate_file]="${2:?}"; shift 2 ;;
      --cache-env) OPT[cache_env]="${2:?}"; shift 2 ;;
      --harness-code-repo) OPT[harness_code_repo]="${2:?}"; shift 2 ;;
      --harness-code-branch) OPT[harness_code_branch]="${2:?}"; shift 2 ;;
      --hc-dir) HC_DIR="${2:?}"; shift 2 ;;
      --jira) JIRA_ID="${2:-}"; shift 2 ;;
      --no-windows) HC_WINDOWS=false; shift ;;
      --run) REMOTE_RUN="${2:?}"; shift 2 ;;
      --execution-url) EXECUTION_URL="${2:?}"; shift 2 ;;
      --result) REMOTE_RESULT="${2:?}"; shift 2 ;;
      --metrics) RUN_METRICS="${2:-}"; shift 2 ;;
      --gate) GATE="${2:?}"; shift 2 ;;
      --reason) REASON="${2:?}"; shift 2 ;;
      --from) FROM_DIR="${2:?}"; shift 2 ;;
      --files) FIXTURE_FILES="${2:-}"; shift 2 ;;
      --local-test) TEST_MODE=local; shift ;;
      --lifecycle) TEST_MODE=lifecycle; shift ;;
      --compatibility) TEST_MODE=compatibility; shift ;;
      --public) TEST_MODE=public; shift ;;
      --all) TEST_MODE=all; shift ;;
      -h|--help) COMMAND=help; shift ;;
      *) die "unknown option: $1" ;;
    esac
  done
}

main() {
  parse_args "$@"
  need python3
  cd "$REPO_ROOT"
  case "$COMMAND" in
    help) cmd_help ;;
    status) cmd_status ;;
    research) cmd_research ;;
    design) cmd_design ;;
    create) cmd_create ;;
    test) cmd_test ;;
    build) cmd_build ;;
    finalize) cmd_finalize ;;
    harness-core) cmd_harness_core ;;
    remote-setup) cmd_remote_setup ;;
    remote-test) cmd_remote_test ;;
    validate) cmd_validate ;;
    report) cmd_report ;;
    waive) cmd_waive ;;
    fixture-commit) cmd_fixture_commit ;;
    all) cmd_all ;;
    *) die "unknown command: $COMMAND (try help)" ;;
  esac
}

main "$@"
