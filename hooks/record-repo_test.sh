#!/usr/bin/env bash
# Test the SessionStart repo-recording hook in isolation.
set -euo pipefail

HOOK="$(cd "$(dirname "$0")" && pwd)/record-repo.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

store="$tmp/store"
export CLAUDE_TOKEN_METRICS_STORE="$store"
map="$store/repo-map.json"

# A real git repo plus a linked worktree, so we can prove the hook folds the
# worktree path back to the origin repo.
mainrepo="$tmp/mainrepo"
mkdir -p "$mainrepo"
git -C "$mainrepo" init -q
git -C "$mainrepo" config user.email t@t.t
git -C "$mainrepo" config user.name t
echo x > "$mainrepo/f.txt"
git -C "$mainrepo" add f.txt
git -C "$mainrepo" commit -q -m init
worktree="$tmp/wt"
git -C "$mainrepo" worktree add -q "$worktree" -b feature

# 1. Worktree cwd resolves to the MAIN repo, not the worktree path.
printf '{"cwd":"%s"}' "$worktree" | bash "$HOOK"
got="$(jq -r --arg k "$worktree" '.[$k]' "$map")"
# git may resolve symlinks (/var -> /private/var); compare canonical paths.
want="$(cd "$mainrepo" && pwd -P)"
gotc="$(cd "$got" && pwd -P)"
if [ "$gotc" != "$want" ]; then
  echo "FAIL: worktree mapped to $got, want $mainrepo" >&2
  exit 1
fi

# 2. A plain repo cwd resolves to itself.
printf '{"cwd":"%s"}' "$mainrepo" | bash "$HOOK"
got2="$(jq -r --arg k "$mainrepo" '.[$k]' "$map")"
gotc2="$(cd "$got2" && pwd -P)"
if [ "$gotc2" != "$want" ]; then
  echo "FAIL: main repo mapped to $got2, want $mainrepo" >&2
  exit 1
fi

# 3. Both entries coexist (the second run did not clobber the first).
if [ "$(jq 'length' "$map")" != "2" ]; then
  echo "FAIL: map has $(jq 'length' "$map") entries, want 2" >&2
  exit 1
fi

# 4. Idempotency: re-running the worktree case keeps the map at 2 entries.
printf '{"cwd":"%s"}' "$worktree" | bash "$HOOK"
if [ "$(jq 'length' "$map")" != "2" ]; then
  echo "FAIL: map grew after idempotent re-run" >&2
  exit 1
fi

# 5. A non-git directory is skipped silently, map unchanged, exit 0.
notrepo="$tmp/notrepo"
mkdir -p "$notrepo"
printf '{"cwd":"%s"}' "$notrepo" | bash "$HOOK"
if [ "$(jq 'length' "$map")" != "2" ]; then
  echo "FAIL: non-git cwd added a map entry" >&2
  exit 1
fi

# 6. Empty payload must exit 0 and not crash.
printf '{}' | bash "$HOOK" || { echo "FAIL: empty payload should exit 0" >&2; exit 1; }

# 7. First-ever run with no existing map file still works (fresh store).
fresh="$tmp/fresh"
export CLAUDE_TOKEN_METRICS_STORE="$fresh"
printf '{"cwd":"%s"}' "$mainrepo" | bash "$HOOK"
if [ ! -f "$fresh/repo-map.json" ]; then
  echo "FAIL: map not created on first run" >&2
  exit 1
fi

echo "PASS: record-repo.sh"
