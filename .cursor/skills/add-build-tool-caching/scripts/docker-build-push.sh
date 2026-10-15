#!/usr/bin/env bash
# Build and push a candidate drone-cache Linux amd64 image to Docker Hub.
# Intended for Harness CI Cloud VM steps (docker daemon + connector/secret available).
# The harness-ai-agent container has no Docker daemon; run this in a pipeline Run step instead.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: docker-build-push.sh --tool <name> [--tag <tag>] [--registry-user <user>]

Environment:
  DOCKER_PASSWORD   Required unless already logged in to the registry.
  REPO_ROOT         Optional; defaults to git root from script location.

Builds release/linux/amd64/drone-cache if missing, then:
  docker build -f docker/Dockerfile.linux.amd64 -t <user>/drone-cache:<tag> .
  docker push <user>/drone-cache:<tag>

Default tag: <tool>-caching-<short-sha>
EOF
}

TOOL=""
TAG=""
REGISTRY_USER="${DOCKER_REGISTRY_USER:-rahulpx9}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tool) TOOL="${2:?}"; shift 2 ;;
    --tag) TAG="${2:?}"; shift 2 ;;
    --registry-user) REGISTRY_USER="${2:?}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "$TOOL" ]]; then
  echo "--tool is required" >&2
  usage
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${REPO_ROOT:-$(cd "$SCRIPT_DIR/../../../.." && pwd)}"
cd "$REPO_ROOT"

if ! command -v docker >/dev/null 2>&1; then
  echo "BLOCKED: docker CLI not available (expected on Harness Cloud VM, not in harness-ai-agent)." >&2
  exit 3
fi

SHORT_SHA="$(git rev-parse --short=12 HEAD)"
TAG="${TAG:-${TOOL}-caching-${SHORT_SHA}}"
IMAGE="${REGISTRY_USER}/drone-cache:${TAG}"

mkdir -p release/linux/amd64
if [[ ! -x release/linux/amd64/drone-cache ]]; then
  echo "=== Cross-compiling release/linux/amd64/drone-cache ==="
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/linux/amd64/drone-cache .
fi

if [[ -n "${DOCKER_PASSWORD:-}" ]]; then
  echo "$DOCKER_PASSWORD" | docker login -u "$REGISTRY_USER" --password-stdin
fi

echo "=== Building ${IMAGE} ==="
docker build -f docker/Dockerfile.linux.amd64 -t "$IMAGE" .

echo "=== Pushing ${IMAGE} ==="
docker push "$IMAGE"

DIGEST="$(docker inspect --format='{{index .RepoDigests 0}}' "$IMAGE" 2>/dev/null || true)"
echo "CANDIDATE_IMAGE=${IMAGE}"
echo "CANDIDATE_DIGEST=${DIGEST}"
