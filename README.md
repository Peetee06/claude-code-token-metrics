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
- copies the SessionEnd hook script into `~/.claude-token-metrics/hooks/`
- merges a `SessionEnd` hook into `~/.claude/settings.json` (existing hooks
  are preserved; re-running is idempotent)
- installs a launchd job that runs `sweep` every 4 hours

## Commands

| Command | What it does |
|---|---|
| `setup` | Install the hook and the launchd sweep job. |
| `sweep` | One-shot rsync of `~/.claude/projects` into the snapshot store. |
| `analyze [--out <dir>]` | Resolve repos, run ccusage, write CSV/JSON (default `out/`). |

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
hook script, and `setup` belong in this repo — never snapshots.**
