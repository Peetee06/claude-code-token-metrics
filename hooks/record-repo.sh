#!/usr/bin/env bash
# SessionStart hook for claude-code-token-metrics.
# Claude Code passes a JSON payload on stdin including the session's `cwd`.
# While the worktree is still alive we resolve its origin git repo and record
# a durable cwd -> origin-repo mapping. Squad deletes worktrees (and their
# state.json entries) when an instance is killed, so capturing the link now
# is the only way analyze can still attribute the session to a repo later.
#
# This script must never fail the host session: every path exits 0.
set -u

STORE="${CLAUDE_TOKEN_METRICS_STORE:-$HOME/.claude-token-metrics}"
MAP="$STORE/repo-map.json"

payload="$(cat)"
cwd="$(printf '%s' "$payload" | jq -r '.cwd // empty')"

if [ -z "$cwd" ] || [ ! -d "$cwd" ]; then
  exit 0
fi

# --git-common-dir points at the SHARED .git dir even from a linked worktree;
# its parent directory is the origin repo root.
common="$(git -C "$cwd" rev-parse --git-common-dir 2>/dev/null)"
if [ -z "$common" ]; then
  # Not a git repo (or git failed) -> nothing to record.
  exit 0
fi
# Make relative results (".git") absolute before taking the parent.
case "$common" in
  /*) ;;
  *) common="$(cd "$cwd" && cd "$common" && pwd)" || exit 0 ;;
esac
repo="$(dirname "$common")"

mkdir -p "$STORE" || exit 0

# Merge { "<cwd>": "<repo>" } into the map. jq -n with --slurpfile tolerates a
# missing/empty map file. Last write wins; re-running is idempotent.
tmp="$(mktemp "${TMPDIR:-/tmp}/cctm-repomap.XXXXXX")" || exit 0
if jq -n --slurpfile existing <(cat "$MAP" 2>/dev/null || echo '{}') \
        --arg cwd "$cwd" --arg repo "$repo" \
        '($existing[0] // {}) + {($cwd): $repo}' > "$tmp" 2>/dev/null; then
  mv -f "$tmp" "$MAP" 2>/dev/null || rm -f "$tmp"
else
  rm -f "$tmp"
fi

exit 0
