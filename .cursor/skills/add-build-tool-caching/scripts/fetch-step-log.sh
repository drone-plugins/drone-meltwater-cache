#!/bin/sh
# Download and print a Harness step log from the signed URL returned by
# harness_get(resource_type="execution_log", params={..., return_download_url: true}).
# QA0 returns a URL behind <host>/storage/harness-download/<bucket>/..., which 404s;
# the same signed object is served by storage.googleapis.com/<bucket>/....
#
# Usage: fetch-step-log.sh '<download_url>' [tail_lines]
set -eu

url="${1:?usage: fetch-step-log.sh <download_url> [tail_lines]}"
lines="${2:-200}"

case "$url" in
  *"/storage/harness-download/"*)
    url="https://storage.googleapis.com/${url#*/storage/harness-download/}"
    ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

code="$(curl -sS -o "$tmp/logs.zip" -w '%{http_code}' "$url")"
if [ "$code" != "200" ]; then
  echo "log download failed: HTTP $code" >&2
  exit 1
fi

if command -v unzip >/dev/null 2>&1; then
  unzip -o -q "$tmp/logs.zip" -d "$tmp/out"
elif command -v python3 >/dev/null 2>&1; then
  python3 -c 'import sys, zipfile; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])' "$tmp/logs.zip" "$tmp/out"
else
  echo "need unzip or python3 to read the log archive" >&2
  exit 1
fi
find "$tmp/out" -type f | while read -r f; do
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$f" <<'PY'
import json, re, sys
ansi = re.compile(r"\x1b\[[0-9;]*m")
for line in open(sys.argv[1], errors="replace"):
    try:
        out = json.loads(line).get("out", "")
    except Exception:
        out = line
    print(ansi.sub("", out.rstrip("\n")))
PY
  else
    sed -e 's/^{"level":"[a-z]*","pos":[0-9]*,"out":"//' -e 's/","time":.*$//' "$f"
  fi
done | tail -n "$lines"
