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
| `report [--dir <dir>]` | Build a self-contained `report.html` chart from analyze output (default `out/`). |

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

## Project identity

Usage is grouped by **project**, not raw repo path. Each resolved repo is
mapped to a clean project name — its git remote as `owner/repo` — so the same
project checked out at several paths (a dev clone, CI-runner working dirs)
collapses into one project. The repo path is kept only as secondary detail.

### Optional config

`~/.claude-token-metrics/config.json` is optional:

```json
{
  "squad_default_repo": "/path/to/your/repo",
  "project_aliases": { "/path/with/no/remote": "owner/project" }
}
```

- `squad_default_repo` — any Claude Squad worktree session that nothing else
  resolves is attributed to this repo. Recovers historical sessions whose
  worktrees were deleted before the `SessionStart` hook was installed.
- `project_aliases` — maps a resolved repo path to an explicit project name,
  overriding git-remote derivation. Useful when a path's worktree is gone (no
  remote to read).

## Output

`analyze` writes four files:

- `sessions.csv` / `sessions.json` — one row per `(project, repo, project_dir, model)`.
- `daily.csv` / `daily.json` — one row per `(date, project, model)`.

`project` is the primary identifier; `repo` is secondary detail. Token columns:
`input_tokens`, `output_tokens`, `cache_creation_tokens`, `cache_read_tokens`,
plus `cost`. Sessions whose repo cannot be resolved are bucketed under
`unknown` — never dropped.

## HTML report

`report` reads `daily.json` and writes a self-contained `report.html` — a
cost/token chart with one line per project, plus a per-project breakdown. It
needs no local assets (charts render via a CDN-loaded library) and opens in any
browser. The trend chart has a filter popover: switch between cost and tokens,
daily and weekly buckets, linear and log scale, and zoom to a date range.

## Privacy

The snapshot store (`~/.claude-token-metrics/snapshots/`) contains your
prompts and source code. It is local-only and gitignored. **Only code, the
hook scripts, and `setup` belong in this repo — never snapshots.**
