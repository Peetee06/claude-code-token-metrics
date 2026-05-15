# claude-code-token-metrics

Durable capture and per-repository analysis of Claude Code token usage.

Existing analytics tools read live `~/.claude/projects/*.jsonl` at runtime.
That breaks for Claude Squad workflows: a finished, killed Squad task deletes
its worktree, orphaning the transcript before it can be analyzed. This tool
**snapshots transcripts before cleanup destroys them**, then groups usage by
the git repository each session belonged to.

## Requirements

- macOS (the sweep job uses launchd)
- `git`, `rsync` (preinstalled on macOS)
- [`ccusage`](https://ccusage.com) — reachable via `npx ccusage` (Node) or a
  global install. The tool runs ccusage for token aggregation.

## Install

```bash
go build -o claude-code-token-metrics .
./claude-code-token-metrics setup
```

`setup`:
- copies the `SessionStart` and `SessionEnd` hook scripts into
  `~/.claude-token-metrics/hooks/`
- merges both hooks into `~/.claude/settings.json` (existing hooks are
  preserved; re-running is idempotent)
- installs a launchd job that runs `sweep` every 4 hours

## Commands

| Command | What it does |
|---|---|
| `setup` | Install the hooks and the launchd sweep job. |
| `sweep` | One-shot rsync of `~/.claude/projects` into the snapshot store, plus a refresh of the repo-map index. |
| `analyze [--out <dir>]` | Resolve repos, run ccusage, write CSV/JSON (default `out/`). |

## Repo resolution

Attributing a transcript to its git repo is hard once a Claude Squad worktree
is deleted: the worktree directory is gone and Squad also removes the instance
from its `state.json`. To survive this, a **`SessionStart` hook records each
session's `cwd` → origin-repo mapping while the worktree is still alive**, into
a durable `~/.claude-token-metrics/repo-map.json`. `sweep` also refreshes that
index from the live `state.json`.

`analyze` resolves each transcript in this order: live `git` lookup → the
durable repo-map → the live `state.json` → an optional Squad-default fallback →
`unknown`.

### Optional config

`~/.claude-token-metrics/config.json` is optional:

```json
{ "squad_default_repo": "/path/to/your/repo" }
```

When set, any Claude Squad worktree session that nothing else resolves is
attributed to `squad_default_repo`. This recovers historical sessions whose
worktrees were deleted before the `SessionStart` hook was installed — useful if
you only ever ran Claude Squad against a single repo.

## Output

`analyze` writes four files:

- `sessions.csv` / `sessions.json` — one row per `(project_dir, repo, model)`.
- `daily.csv` / `daily.json` — one row per `(date, repo, model)`.

Token columns: `input_tokens`, `output_tokens`, `cache_creation_tokens`,
`cache_read_tokens`, plus `cost`. Sessions whose repo cannot be resolved are
bucketed under `unknown` — never dropped.

## Privacy

The snapshot store (`~/.claude-token-metrics/snapshots/`) contains your
prompts and source code. It is local-only and gitignored. **Only code, the
hook scripts, and `setup` belong in this repo — never snapshots.**
