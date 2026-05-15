#!/usr/bin/env bash
# SessionEnd hook for claude-code-token-metrics.
# Claude Code passes a JSON payload on stdin containing the transcript path.
# We copy that transcript into the durable snapshot store, preserving the
# source project-dir name so repo resolution works later.
#
# This script must never fail the host session: every path logs and exits 0.
set -u

STORE="${CLAUDE_TOKEN_METRICS_STORE:-$HOME/.claude-token-metrics/snapshots}"

payload="$(cat)"
transcript="$(printf '%s' "$payload" | jq -r '.transcript_path // empty')"

if [ -z "$transcript" ] || [ ! -f "$transcript" ]; then
  # Nothing to copy; not an error for our purposes.
  exit 0
fi

# The transcript lives at <claude>/projects/<project-dir>/<uuid>.jsonl.
# Preserve <project-dir>/<uuid>.jsonl under the store's projects/ dir.
project_dir="$(basename "$(dirname "$transcript")")"
dest_dir="$STORE/projects/$project_dir"

mkdir -p "$dest_dir" || exit 0
cp -f "$transcript" "$dest_dir/" || exit 0

exit 0
