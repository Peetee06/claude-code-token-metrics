#!/usr/bin/env bash
# Test the SessionEnd hook script in isolation.
set -euo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/snapshot-transcript.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Build a fake transcript under a fake claude projects dir.
proj="$tmp/claude/projects/-Users-me-dev-Iakuvo"
mkdir -p "$proj"
transcript="$proj/abc123.jsonl"
printf '{"type":"system"}\n' > "$transcript"

store="$tmp/store"
export CLAUDE_TOKEN_METRICS_STORE="$store"

# Invoke the hook with a Claude-Code-style stdin payload.
printf '{"transcript_path":"%s"}' "$transcript" | bash "$HOOK"

dest="$store/projects/-Users-me-dev-Iakuvo/abc123.jsonl"
if [ ! -f "$dest" ]; then
  echo "FAIL: transcript not copied to $dest" >&2
  exit 1
fi
if [ "$(cat "$dest")" != '{"type":"system"}' ]; then
  echo "FAIL: copied content mismatch" >&2
  exit 1
fi

# Idempotency: running again must not error and must keep the file.
printf '{"transcript_path":"%s"}' "$transcript" | bash "$HOOK"
[ -f "$dest" ] || { echo "FAIL: file gone after second run" >&2; exit 1; }

# Missing transcript path must exit 0 (never break the host session).
printf '{}' | bash "$HOOK" || { echo "FAIL: empty payload should exit 0" >&2; exit 1; }

echo "PASS: snapshot-transcript.sh"
