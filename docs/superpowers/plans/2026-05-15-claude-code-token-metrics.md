# claude-code-token-metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI that durably snapshots Claude Code transcripts before Claude Squad / Claude Code delete their worktrees, then resolves each transcript's git repo and exports token-usage CSV/JSON grouped by repo.

**Architecture:** A single static Go binary `claude-code-token-metrics` with three subcommands. `setup` installs a `SessionEnd` hook into `~/.claude/settings.json` and a launchd sweep job. `sweep` rsyncs `~/.claude/projects` into a local snapshot store. `analyze` resolves each project directory to a canonical git repo, runs `ccusage` against the snapshot store, and joins ccusage's per-(date, project, model) output to repo groups, emitting CSV + JSON. The snapshot store is local-only and never committed.

**Tech Stack:** Go 1.26 (stdlib only — `encoding/json`, `encoding/csv`, `os/exec`, `flag`); `ccusage` (npm, invoked as a subprocess); `rsync`; `git`; `launchd`.

---

## Background facts (verified against this machine, 2026-05-15)

These were checked against real data and a real `ccusage` run. They override conflicting statements in `docs/2026-05-15-design.md`.

1. **ccusage has no custom-data-directory flag.** It reads the `CLAUDE_CONFIG_DIR` environment variable and expects a `projects/` subdirectory inside it. Therefore the snapshot store root must contain a `projects/` directory: transcripts live at `<store>/projects/<project-dir>/<uuid>.jsonl`, and `analyze` invokes ccusage with `CLAUDE_CONFIG_DIR=<store>`.

2. **`cwd` is NOT present on every transcript line.** Real transcripts contain mixed line types (`ai-title`, `system`, `attachment`, `assistant`, `user`, `file-history-snapshot`, etc.). `cwd` appears only on some types (e.g. `assistant`, `user`). Repo resolution must scan for the **first line that has a non-empty `cwd`**, not assume line 1.

3. **ccusage's `session` report is keyed by project directory, not by transcript UUID.** A single "session" entry has `sessionId` like `-Users-petertrost-dev-Iakuvo` and aggregates every transcript in that directory. ccusage does not expose a per-transcript-UUID breakdown. Consequently the design's "one row per transcript" `sessions.csv` is not obtainable. **Redefinition:** `sessions.csv`/`sessions.json` become **one row per (project-dir, repo, model)** — the finest grain ccusage exposes, from `ccusage session --instances --breakdown --json`. The `session_id` column is renamed `project_dir` and carries the ccusage project key.

4. **The grain for the daily rollup is available directly.** `ccusage daily --instances --breakdown --json` returns:
   ```json
   { "projects": { "<project-dir>": [ { "date": "...", "inputTokens": 0, "outputTokens": 0,
       "cacheCreationTokens": 0, "cacheReadTokens": 0, "totalCost": 0.0,
       "modelBreakdowns": [ { "modelName": "...", "inputTokens": 0, "outputTokens": 0,
         "cacheCreationTokens": 0, "cacheReadTokens": 0, "cost": 0.0 } ] } ] } },
     "totals": { ... } }
   ```
   `ccusage session --instances --breakdown --json` returns the same `modelBreakdowns` shape under `{ "sessions": [ { "sessionId": "<project-dir>", ..., "modelBreakdowns": [...] } ] }`.

## File structure

All paths relative to repo root `/Users/petertrost/dev/claude-code-token-metrics`.

- `go.mod` — module `github.com/petertrost/claude-code-token-metrics`, Go 1.26.
- `main.go` — entrypoint; subcommand dispatch (`setup`, `sweep`, `analyze`); `--help`.
- `internal/paths/paths.go` — well-known path helpers (claude dir, store root, store `projects/` dir, squad state path). Single source of truth so tests can override the home dir.
- `internal/snapshot/snapshot.go` — `sweep` logic: rsync `~/.claude/projects/` into `<store>/projects/`.
- `internal/repo/resolve.go` — repo-resolution heuristic: project-dir name + transcript `cwd` + squad state → canonical origin-repo path.
- `internal/repo/squad.go` — parse `~/.claude-squad/state.json` into a lookup of `worktree_path → repo_path`.
- `internal/ccusage/ccusage.go` — invoke `ccusage`, parse the two `--json` payloads into Go structs.
- `internal/analyze/analyze.go` — orchestrate `analyze`: resolve repos, call ccusage, join, build rows.
- `internal/export/export.go` — write `sessions.{csv,json}` and `daily.{csv,json}`.
- `internal/setup/setup.go` — merge the `SessionEnd` hook into `settings.json`; install launchd plist.
- `hooks/snapshot-transcript.sh` — the `SessionEnd` hook script (copied verbatim by `setup`).
- Tests live next to code as `*_test.go`; fixtures under `internal/<pkg>/testdata/`.

The hook script is shell, not Go, because Claude Code invokes hooks as shell commands and a tiny `cp` script needs no binary on `PATH`.

---

## Task 1: Repo scaffold and module

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `internal/paths/paths.go`

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go mod init github.com/petertrost/claude-code-token-metrics
```
Expected: creates `go.mod` with `go 1.26` (or `go 1.26.2`).

- [ ] **Step 2: Create the paths helper**

Create `internal/paths/paths.go`:
```go
// Package paths centralizes the well-known filesystem locations the tool uses.
// HomeDir is a package variable so tests can point it at a temp directory.
package paths

import (
	"os"
	"path/filepath"
)

// HomeDir returns the user's home directory. Override in tests.
var HomeDir = func() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// ClaudeProjectsDir is the live Claude Code transcript directory.
func ClaudeProjectsDir() string {
	return filepath.Join(HomeDir(), ".claude", "projects")
}

// ClaudeSettingsFile is the Claude Code settings file edited by `setup`.
func ClaudeSettingsFile() string {
	return filepath.Join(HomeDir(), ".claude", "settings.json")
}

// StoreRoot is the snapshot store root. ccusage is invoked with
// CLAUDE_CONFIG_DIR set to this path, so it must contain a projects/ subdir.
func StoreRoot() string {
	return filepath.Join(HomeDir(), ".claude-token-metrics", "snapshots")
}

// StoreProjectsDir is the projects/ subdirectory ccusage expects.
func StoreProjectsDir() string {
	return filepath.Join(StoreRoot(), "projects")
}

// SquadStateFile is the Claude Squad state file used for worktree resolution.
func SquadStateFile() string {
	return filepath.Join(HomeDir(), ".claude-squad", "state.json")
}
```

- [ ] **Step 3: Create a minimal main with subcommand dispatch**

Create `main.go`:
```go
package main

import (
	"fmt"
	"os"
)

const usage = `claude-code-token-metrics — durable Claude Code token-usage capture & analysis

Usage:
  claude-code-token-metrics setup     Install the SessionEnd hook and launchd sweep job
  claude-code-token-metrics sweep     Snapshot ~/.claude/projects into the local store
  claude-code-token-metrics analyze   Resolve repos, run ccusage, write CSV/JSON
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "setup":
		fmt.Println("setup: not yet implemented")
	case "sweep":
		fmt.Println("sweep: not yet implemented")
	case "analyze":
		fmt.Println("analyze: not yet implemented")
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
```

- [ ] **Step 4: Verify it builds and runs**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go build ./... && go run . --help
```
Expected: build succeeds; `--help` prints the usage block.

- [ ] **Step 5: Commit**

```bash
git add go.mod main.go internal/paths/paths.go
git commit -m "feat: scaffold Go module with subcommand dispatch"
```

---

## Task 2: Squad state parsing

**Files:**
- Create: `internal/repo/squad.go`
- Test: `internal/repo/squad_test.go`
- Create: `internal/repo/testdata/squad-state.json`

- [ ] **Step 1: Write the fixture**

Create `internal/repo/testdata/squad-state.json` (a trimmed real shape):
```json
{
  "instances": [
    {
      "title": "fix/archived-inspections",
      "branch": "fix/archived-inspections",
      "worktree": {
        "repo_path": "/Users/me/dev/Iakuvo",
        "worktree_path": "/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0",
        "branch_name": "fix/archived-inspections"
      }
    },
    {
      "title": "build-ast-tool",
      "branch": "build-ast-tool",
      "worktree": {
        "repo_path": "/Users/me/dev/other-repo",
        "worktree_path": "/Users/me/.claude-squad/worktrees/build-ast-tool_18af8b8b8eb18d58",
        "branch_name": "build-ast-tool"
      }
    }
  ]
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/repo/squad_test.go`:
```go
package repo

import "testing"

func TestLoadSquadState(t *testing.T) {
	idx, err := LoadSquadState("testdata/squad-state.json")
	if err != nil {
		t.Fatalf("LoadSquadState: %v", err)
	}
	got := idx["/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0"]
	if got != "/Users/me/dev/Iakuvo" {
		t.Errorf("worktree lookup = %q, want /Users/me/dev/Iakuvo", got)
	}
	if len(idx) != 2 {
		t.Errorf("index size = %d, want 2", len(idx))
	}
}

func TestLoadSquadStateMissingFile(t *testing.T) {
	idx, err := LoadSquadState("testdata/does-not-exist.json")
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if idx == nil {
		t.Error("missing file should yield an empty (non-nil) index")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestLoadSquadState -v`
Expected: FAIL — `undefined: LoadSquadState`.

- [ ] **Step 4: Implement squad parsing**

Create `internal/repo/squad.go`:
```go
package repo

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// squadState mirrors the subset of ~/.claude-squad/state.json we use.
type squadState struct {
	Instances []struct {
		Worktree struct {
			RepoPath     string `json:"repo_path"`
			WorktreePath string `json:"worktree_path"`
		} `json:"worktree"`
	} `json:"instances"`
}

// LoadSquadState reads the Claude Squad state file and returns a lookup of
// worktree_path -> repo_path. A missing file yields an empty index and no
// error, so callers can run on machines without Claude Squad.
func LoadSquadState(path string) (map[string]string, error) {
	idx := map[string]string{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	var s squadState
	if err := json.Unmarshal(data, &s); err != nil {
		return idx, err
	}
	for _, inst := range s.Instances {
		wt := inst.Worktree
		if wt.WorktreePath != "" && wt.RepoPath != "" {
			idx[wt.WorktreePath] = wt.RepoPath
		}
	}
	return idx, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestLoadSquadState -v`
Expected: PASS — both tests.

- [ ] **Step 6: Commit**

```bash
git add internal/repo/squad.go internal/repo/squad_test.go internal/repo/testdata/squad-state.json
git commit -m "feat: parse claude-squad state for worktree-to-repo lookup"
```

---

## Task 3: Read first cwd from a transcript

**Files:**
- Create: `internal/repo/transcript.go`
- Test: `internal/repo/transcript_test.go`
- Create: `internal/repo/testdata/transcript-mixed.jsonl`
- Create: `internal/repo/testdata/transcript-nocwd.jsonl`

- [ ] **Step 1: Write the fixtures**

Create `internal/repo/testdata/transcript-mixed.jsonl` — first lines have no `cwd`, a later line does:
```
{"type":"ai-title","sessionId":"s1"}
{"type":"system","sessionId":"s1"}
{"type":"assistant","sessionId":"s1","cwd":"/Users/me/dev/Iakuvo","message":{"model":"claude-opus-4-7"}}
{"type":"user","sessionId":"s1","cwd":"/Users/me/dev/Iakuvo"}
```

Create `internal/repo/testdata/transcript-nocwd.jsonl` — no line has `cwd`:
```
{"type":"ai-title","sessionId":"s2"}
{"type":"system","sessionId":"s2"}
```

- [ ] **Step 2: Write the failing test**

Create `internal/repo/transcript_test.go`:
```go
package repo

import "testing"

func TestFirstCwd(t *testing.T) {
	got, err := FirstCwd("testdata/transcript-mixed.jsonl")
	if err != nil {
		t.Fatalf("FirstCwd: %v", err)
	}
	if got != "/Users/me/dev/Iakuvo" {
		t.Errorf("FirstCwd = %q, want /Users/me/dev/Iakuvo", got)
	}
}

func TestFirstCwdNoneFound(t *testing.T) {
	got, err := FirstCwd("testdata/transcript-nocwd.jsonl")
	if err != nil {
		t.Fatalf("FirstCwd: %v", err)
	}
	if got != "" {
		t.Errorf("FirstCwd = %q, want empty string when no cwd present", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestFirstCwd -v`
Expected: FAIL — `undefined: FirstCwd`.

- [ ] **Step 4: Implement FirstCwd**

Create `internal/repo/transcript.go`:
```go
package repo

import (
	"bufio"
	"encoding/json"
	"os"
)

// FirstCwd scans a transcript JSONL file and returns the cwd value from the
// first line that carries a non-empty cwd field. Returns "" if no line has
// one. Malformed lines are skipped silently — analyze counts them separately.
func FirstCwd(transcriptPath string) (string, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Transcript lines can be large; raise the line-size ceiling to 8 MiB.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var line struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue // skip malformed line
		}
		if line.Cwd != "" {
			return line.Cwd, nil
		}
	}
	return "", sc.Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestFirstCwd -v`
Expected: PASS — both tests.

- [ ] **Step 6: Commit**

```bash
git add internal/repo/transcript.go internal/repo/transcript_test.go internal/repo/testdata/transcript-mixed.jsonl internal/repo/testdata/transcript-nocwd.jsonl
git commit -m "feat: extract first cwd from a transcript file"
```

---

## Task 4: Path normalization for dead Squad worktrees

This is the pure-string fallback used when a worktree directory no longer exists on disk.

**Files:**
- Create: `internal/repo/normalize.go`
- Test: `internal/repo/normalize_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/repo/normalize_test.go`:
```go
package repo

import "testing"

func TestNormalizeWorktreePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "squad worktree with branch segment and hash suffix",
			in:   "/Users/me/.claude-squad/worktrees/fix/drift-migrations_18afaafbb0011223",
			want: "fix/drift-migrations",
		},
		{
			name: "squad worktree single-segment branch",
			in:   "/Users/me/.claude-squad/worktrees/build-ast-tool_18af8b8b8eb18d58",
			want: "build-ast-tool",
		},
		{
			name: "non-squad path returns empty",
			in:   "/Users/me/dev/Iakuvo",
			want: "",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeWorktreeBranch(c.in); got != c.want {
				t.Errorf("NormalizeWorktreeBranch(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestNormalizeWorktreePath -v`
Expected: FAIL — `undefined: NormalizeWorktreeBranch`.

- [ ] **Step 3: Implement normalization**

Create `internal/repo/normalize.go`:
```go
package repo

import (
	"regexp"
	"strings"
)

// hashSuffix matches the trailing _<hex> Claude Squad appends to worktree dirs.
var hashSuffix = regexp.MustCompile(`_[0-9a-f]{8,}$`)

const squadMarker = "/.claude-squad/worktrees/"

// NormalizeWorktreeBranch extracts the branch name from a Claude Squad
// worktree path by taking everything after ".claude-squad/worktrees/" and
// stripping the trailing _<hash> suffix. Returns "" for any path that is not
// a Claude Squad worktree. The branch may contain slashes (e.g. "fix/x").
func NormalizeWorktreeBranch(path string) string {
	i := strings.Index(path, squadMarker)
	if i < 0 {
		return ""
	}
	rest := path[i+len(squadMarker):]
	rest = hashSuffix.ReplaceAllString(rest, "")
	return rest
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestNormalizeWorktreePath -v`
Expected: PASS — all four subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/repo/normalize.go internal/repo/normalize_test.go
git commit -m "feat: normalize claude-squad worktree paths to branch names"
```

---

## Task 5: Repo resolution orchestrator

Joins the previous three pieces into the full heuristic. Produces a canonical repo path or the string `unknown`.

**Files:**
- Create: `internal/repo/resolve.go`
- Test: `internal/repo/resolve_test.go`
- Create: `internal/repo/testdata/transcript-dead-worktree.jsonl`
- Create: `internal/repo/testdata/transcript-live-repo.jsonl`

- [ ] **Step 1: Write the fixtures**

Create `internal/repo/testdata/transcript-dead-worktree.jsonl` — cwd is a Squad worktree that matches the squad-state fixture:
```
{"type":"system","sessionId":"d1"}
{"type":"assistant","sessionId":"d1","cwd":"/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0"}
```

Create `internal/repo/testdata/transcript-live-repo.jsonl` — cwd is whatever temp dir the test sets up; the test writes this file itself, so this fixture file is only a placeholder and is NOT needed. **Skip creating this file** — the live-repo test builds its transcript in a temp dir at runtime (see Step 2).

- [ ] **Step 2: Write the failing test**

Create `internal/repo/resolve_test.go`:
```go
package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeTranscript writes a one-line transcript whose cwd is dir.
func writeTranscript(t *testing.T, dir, cwd string) string {
	t.Helper()
	p := filepath.Join(dir, "t.jsonl")
	line := `{"type":"assistant","cwd":` + jsonString(cwd) + `}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// jsonString quotes s as a JSON string literal.
func jsonString(s string) string {
	b, _ := os.Getwd() // unused; keep import tidy
	_ = b
	return `"` + s + `"`
}

func TestResolveLiveGitRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t.t")
	run("config", "user.name", "t")

	transcript := writeTranscript(t, dir, dir)
	idx := map[string]string{}

	resolver := NewResolver(idx)
	got := resolver.Resolve("project-dir-name", transcript)
	// git rev-parse may resolve symlinks (e.g. /var -> /private/var on macOS);
	// compare via filepath.EvalSymlinks.
	want, _ := filepath.EvalSymlinks(dir)
	gotEval, _ := filepath.EvalSymlinks(got)
	if gotEval != want {
		t.Errorf("Resolve live repo = %q, want %q", got, want)
	}
}

func TestResolveDeadWorktreeViaSquadState(t *testing.T) {
	idx := map[string]string{
		"/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0": "/Users/me/dev/Iakuvo",
	}
	resolver := NewResolver(idx)
	got := resolver.Resolve("project-dir", "testdata/transcript-dead-worktree.jsonl")
	if got != "/Users/me/dev/Iakuvo" {
		t.Errorf("Resolve dead worktree = %q, want /Users/me/dev/Iakuvo", got)
	}
}

func TestResolveUnknown(t *testing.T) {
	resolver := NewResolver(map[string]string{})
	got := resolver.Resolve("project-dir", "testdata/transcript-nocwd.jsonl")
	if got != "unknown" {
		t.Errorf("Resolve with no cwd = %q, want unknown", got)
	}
}
```

> Note: `jsonString` above is deliberately trivial because the test only ever passes filesystem paths with no characters needing JSON escaping. If a future fixture path contains a quote or backslash, switch to `encoding/json.Marshal`.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestResolve -v`
Expected: FAIL — `undefined: NewResolver`.

- [ ] **Step 4: Implement the resolver**

Create `internal/repo/resolve.go`:
```go
package repo

import (
	"os"
	"os/exec"
	"strings"
)

// UnknownRepo is the bucket for transcripts whose repo cannot be resolved.
const UnknownRepo = "unknown"

// Resolver resolves transcripts to canonical origin-repo paths.
type Resolver struct {
	// squadIdx maps worktree_path -> repo_path from claude-squad state.
	squadIdx map[string]string
}

// NewResolver builds a Resolver from a squad worktree->repo index
// (see LoadSquadState). Pass an empty map if Claude Squad is not used.
func NewResolver(squadIdx map[string]string) *Resolver {
	return &Resolver{squadIdx: squadIdx}
}

// Resolve returns the canonical origin-repo path for one transcript.
// projectDir is the snapshot project-dir name (currently unused but kept for
// future heuristics and a stable signature). transcriptPath is the JSONL file.
// Resolution order:
//  1. Read the first cwd from the transcript.
//  2. If cwd is a live path inside a git repo -> git common dir's parent
//     (folds a worktree back to its origin repo).
//  3. If cwd is dead -> exact match in the squad index.
//  4. Otherwise -> UnknownRepo.
func (r *Resolver) Resolve(projectDir, transcriptPath string) string {
	cwd, err := FirstCwd(transcriptPath)
	if err != nil || cwd == "" {
		return UnknownRepo
	}

	// Step 2: live path -> git.
	if _, statErr := os.Stat(cwd); statErr == nil {
		if repo := gitOriginRepo(cwd); repo != "" {
			return repo
		}
	}

	// Step 3: dead worktree -> squad index exact match.
	if repo, ok := r.squadIdx[cwd]; ok {
		return repo
	}

	return UnknownRepo
}

// gitOriginRepo returns the origin-repo top-level for a live directory,
// folding a worktree back to its main repo. Returns "" on any git failure.
func gitOriginRepo(dir string) string {
	// --git-common-dir points at the SHARED .git dir even from a worktree.
	common := gitOutput(dir, "rev-parse", "--git-common-dir")
	if common == "" {
		return ""
	}
	// common is typically ".git" or an absolute path ending in .git.
	// Resolve it to an absolute path, then take its parent = repo root.
	if !strings.HasPrefix(common, "/") {
		// relative to dir
		toplevel := gitOutput(dir, "rev-parse", "--show-toplevel")
		return toplevel
	}
	// absolute .git path -> strip trailing /.git
	return strings.TrimSuffix(common, "/.git")
}

// gitOutput runs `git -C dir <args...>` and returns trimmed stdout, or "".
func gitOutput(dir string, args ...string) string {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -run TestResolve -v`
Expected: PASS — all three tests.

- [ ] **Step 6: Run the whole repo package**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/repo/ -v`
Expected: PASS — every test in the package.

- [ ] **Step 7: Commit**

```bash
git add internal/repo/resolve.go internal/repo/resolve_test.go internal/repo/testdata/transcript-dead-worktree.jsonl
git commit -m "feat: repo-resolution heuristic joining cwd, git, and squad state"
```

---

## Task 6: Sweep — rsync transcripts into the store

**Files:**
- Create: `internal/snapshot/snapshot.go`
- Test: `internal/snapshot/snapshot_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/snapshot/snapshot_test.go`:
```go
package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSweepCopiesTranscriptsPreservingDirs(t *testing.T) {
	src := t.TempDir()
	dstProjects := filepath.Join(t.TempDir(), "projects")

	// Build a fake ~/.claude/projects with two project dirs.
	projA := filepath.Join(src, "-Users-me-dev-Iakuvo")
	projB := filepath.Join(src, "-Users-me-squad-worktrees-x_abc")
	for _, d := range []string{projA, projB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projA, "s1.jsonl"), []byte(`{"a":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projB, "s2.jsonl"), []byte(`{"b":2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sweep(src, dstProjects); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dstProjects, "-Users-me-dev-Iakuvo", "s1.jsonl"))
	if err != nil {
		t.Fatalf("expected s1.jsonl in store: %v", err)
	}
	if string(got) != `{"a":1}`+"\n" {
		t.Errorf("s1.jsonl content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dstProjects, "-Users-me-squad-worktrees-x_abc", "s2.jsonl")); err != nil {
		t.Errorf("expected s2.jsonl in store: %v", err)
	}
}

func TestSweepIsIdempotent(t *testing.T) {
	src := t.TempDir()
	dstProjects := filepath.Join(t.TempDir(), "projects")
	proj := filepath.Join(src, "p")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "s.jsonl"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Sweep(src, dstProjects); err != nil {
		t.Fatalf("first Sweep: %v", err)
	}
	if err := Sweep(src, dstProjects); err != nil {
		t.Fatalf("second Sweep: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dstProjects, "p", "s.jsonl"))
	if err != nil || string(got) != "x\n" {
		t.Errorf("after two sweeps got %q, err %v", got, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/snapshot/ -v`
Expected: FAIL — `undefined: Sweep`.

- [ ] **Step 3: Implement Sweep**

Create `internal/snapshot/snapshot.go`:
```go
// Package snapshot copies live Claude Code transcripts into the durable store.
package snapshot

import (
	"fmt"
	"os"
	"os/exec"
)

// Sweep mirrors srcProjects (typically ~/.claude/projects) into dstProjects
// (the store's projects/ directory) using rsync. It is additive and
// idempotent: rsync copies only changed files and the tool never deletes
// snapshots, so --delete is intentionally NOT passed.
func Sweep(srcProjects, dstProjects string) error {
	if _, err := os.Stat(srcProjects); err != nil {
		return fmt.Errorf("source projects dir %q not found: %w", srcProjects, err)
	}
	if err := os.MkdirAll(dstProjects, 0o755); err != nil {
		return fmt.Errorf("creating store dir %q: %w", dstProjects, err)
	}
	// Trailing slash on src copies the *contents* of srcProjects into
	// dstProjects, preserving the per-project subdirectory structure.
	cmd := exec.Command("rsync", "-a",
		"--include=*/", "--include=*.jsonl", "--exclude=*",
		srcProjects+"/", dstProjects+"/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w\n%s", err, out)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/snapshot/ -v`
Expected: PASS — both tests.

- [ ] **Step 5: Wire `sweep` into main**

In `main.go`, replace the `case "sweep":` body:
```go
	case "sweep":
		if err := runSweep(); err != nil {
			fmt.Fprintln(os.Stderr, "sweep:", err)
			os.Exit(1)
		}
		fmt.Println("sweep: snapshot store updated")
```
Add to `main.go` (new imports `internal/paths` and `internal/snapshot`):
```go
func runSweep() error {
	return snapshot.Sweep(paths.ClaudeProjectsDir(), paths.StoreProjectsDir())
}
```
Adjust the import block at the top of `main.go`:
```go
import (
	"fmt"
	"os"

	"github.com/petertrost/claude-code-token-metrics/internal/paths"
	"github.com/petertrost/claude-code-token-metrics/internal/snapshot"
)
```

- [ ] **Step 6: Verify build and a real sweep**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go build ./... && go run . sweep && ls ~/.claude-token-metrics/snapshots/projects | head -3
```
Expected: build succeeds; `sweep` prints success; the store's `projects/` lists real project directories.

- [ ] **Step 7: Commit**

```bash
git add internal/snapshot/snapshot.go internal/snapshot/snapshot_test.go main.go
git commit -m "feat: sweep transcripts into the durable snapshot store"
```

---

## Task 7: ccusage invocation and JSON parsing

**Files:**
- Create: `internal/ccusage/ccusage.go`
- Test: `internal/ccusage/ccusage_test.go`
- Create: `internal/ccusage/testdata/daily.json`
- Create: `internal/ccusage/testdata/session.json`

- [ ] **Step 1: Write the fixtures**

Create `internal/ccusage/testdata/daily.json` (real shape from `ccusage daily --instances --breakdown --json`):
```json
{
  "projects": {
    "-Users-me-dev-Iakuvo": [
      {
        "date": "2026-05-15",
        "inputTokens": 100,
        "outputTokens": 200,
        "cacheCreationTokens": 30,
        "cacheReadTokens": 40,
        "totalCost": 1.5,
        "modelBreakdowns": [
          { "modelName": "claude-opus-4-7", "inputTokens": 60, "outputTokens": 120,
            "cacheCreationTokens": 20, "cacheReadTokens": 25, "cost": 1.0 },
          { "modelName": "claude-sonnet-4-6", "inputTokens": 40, "outputTokens": 80,
            "cacheCreationTokens": 10, "cacheReadTokens": 15, "cost": 0.5 }
        ]
      }
    ],
    "-Users-me-squad-worktrees-x_abc": [
      {
        "date": "2026-05-14",
        "inputTokens": 5,
        "outputTokens": 6,
        "cacheCreationTokens": 1,
        "cacheReadTokens": 2,
        "totalCost": 0.1,
        "modelBreakdowns": [
          { "modelName": "claude-haiku-4-5-20251001", "inputTokens": 5, "outputTokens": 6,
            "cacheCreationTokens": 1, "cacheReadTokens": 2, "cost": 0.1 }
        ]
      }
    ]
  },
  "totals": {}
}
```

Create `internal/ccusage/testdata/session.json` (real shape from `ccusage session --instances --breakdown --json`):
```json
{
  "sessions": [
    {
      "sessionId": "-Users-me-dev-Iakuvo",
      "lastActivity": "2026-05-15",
      "totalCost": 1.5,
      "modelBreakdowns": [
        { "modelName": "claude-opus-4-7", "inputTokens": 60, "outputTokens": 120,
          "cacheCreationTokens": 20, "cacheReadTokens": 25, "cost": 1.0 },
        { "modelName": "claude-sonnet-4-6", "inputTokens": 40, "outputTokens": 80,
          "cacheCreationTokens": 10, "cacheReadTokens": 15, "cost": 0.5 }
      ]
    }
  ],
  "totals": {}
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/ccusage/ccusage_test.go`:
```go
package ccusage

import (
	"os"
	"testing"
)

func TestParseDaily(t *testing.T) {
	data, err := os.ReadFile("testdata/daily.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ParseDaily(data)
	if err != nil {
		t.Fatalf("ParseDaily: %v", err)
	}
	rows := rep.Projects["-Users-me-dev-Iakuvo"]
	if len(rows) != 1 {
		t.Fatalf("Iakuvo day count = %d, want 1", len(rows))
	}
	if rows[0].Date != "2026-05-15" {
		t.Errorf("date = %q, want 2026-05-15", rows[0].Date)
	}
	if len(rows[0].ModelBreakdowns) != 2 {
		t.Fatalf("model breakdown count = %d, want 2", len(rows[0].ModelBreakdowns))
	}
	mb := rows[0].ModelBreakdowns[0]
	if mb.ModelName != "claude-opus-4-7" || mb.InputTokens != 60 || mb.Cost != 1.0 {
		t.Errorf("first model breakdown = %+v", mb)
	}
}

func TestParseSession(t *testing.T) {
	data, err := os.ReadFile("testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ParseSession(data)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if len(rep.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(rep.Sessions))
	}
	s := rep.Sessions[0]
	if s.SessionID != "-Users-me-dev-Iakuvo" {
		t.Errorf("sessionId = %q", s.SessionID)
	}
	if len(s.ModelBreakdowns) != 2 {
		t.Errorf("model breakdown count = %d, want 2", len(s.ModelBreakdowns))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/ccusage/ -v`
Expected: FAIL — `undefined: ParseDaily`.

- [ ] **Step 4: Implement parsing and the runner**

Create `internal/ccusage/ccusage.go`:
```go
// Package ccusage invokes the ccusage CLI and parses its JSON reports.
package ccusage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// ModelBreakdown is one model's token/cost contribution within a row.
// Field names match ccusage's JSON exactly.
type ModelBreakdown struct {
	ModelName           string  `json:"modelName"`
	InputTokens         int64   `json:"inputTokens"`
	OutputTokens        int64   `json:"outputTokens"`
	CacheCreationTokens int64   `json:"cacheCreationTokens"`
	CacheReadTokens     int64   `json:"cacheReadTokens"`
	Cost                float64 `json:"cost"`
}

// DailyRow is one project's usage on one date.
type DailyRow struct {
	Date            string           `json:"date"`
	ModelBreakdowns []ModelBreakdown `json:"modelBreakdowns"`
}

// DailyReport is the parsed `ccusage daily --instances --breakdown --json`.
// Projects maps a ccusage project-dir key to its daily rows.
type DailyReport struct {
	Projects map[string][]DailyRow `json:"projects"`
}

// SessionEntry is one project's lifetime usage (ccusage keys sessions by
// project directory, not by transcript UUID).
type SessionEntry struct {
	SessionID       string           `json:"sessionId"`
	LastActivity    string           `json:"lastActivity"`
	ModelBreakdowns []ModelBreakdown `json:"modelBreakdowns"`
}

// SessionReport is the parsed `ccusage session --instances --breakdown --json`.
type SessionReport struct {
	Sessions []SessionEntry `json:"sessions"`
}

// ParseDaily parses a ccusage daily JSON payload.
func ParseDaily(data []byte) (*DailyReport, error) {
	var r DailyReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing ccusage daily JSON: %w", err)
	}
	return &r, nil
}

// ParseSession parses a ccusage session JSON payload.
func ParseSession(data []byte) (*SessionReport, error) {
	var r SessionReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing ccusage session JSON: %w", err)
	}
	return &r, nil
}

// ErrNotInstalled is returned when the ccusage binary cannot be launched.
var ErrNotInstalled = errors.New(
	"ccusage not found — install it with `npm install -g ccusage` " +
		"(or ensure `npx ccusage` works) and re-run")

// Runner invokes ccusage. The Command field is the executable plus any
// leading args; for `npx ccusage` use []string{"npx", "ccusage"}.
type Runner struct {
	Command []string // e.g. ["ccusage"] or ["npx", "ccusage"]
}

// DefaultRunner uses `npx ccusage` so the tool works without a global install.
func DefaultRunner() *Runner {
	return &Runner{Command: []string{"npx", "ccusage"}}
}

// run executes ccusage with CLAUDE_CONFIG_DIR=storeRoot and the given args,
// returning stdout. A launch failure is reported as ErrNotInstalled.
func (r *Runner) run(storeRoot string, args ...string) ([]byte, error) {
	full := append(append([]string{}, r.Command[1:]...), args...)
	cmd := exec.Command(r.Command[0], full...)
	cmd.Env = append(cmd.Environ(), "CLAUDE_CONFIG_DIR="+storeRoot)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("ccusage exited %d: %s",
				exitErr.ExitCode(), exitErr.Stderr)
		}
		return nil, ErrNotInstalled
	}
	return out, nil
}

// Daily runs `ccusage daily --instances --breakdown --json`.
func (r *Runner) Daily(storeRoot string) (*DailyReport, error) {
	out, err := r.run(storeRoot,
		"daily", "--instances", "--breakdown", "--json")
	if err != nil {
		return nil, err
	}
	return ParseDaily(out)
}

// Session runs `ccusage session --instances --breakdown --json`.
func (r *Runner) Session(storeRoot string) (*SessionReport, error) {
	out, err := r.run(storeRoot,
		"session", "--instances", "--breakdown", "--json")
	if err != nil {
		return nil, err
	}
	return ParseSession(out)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/ccusage/ -v`
Expected: PASS — both tests.

- [ ] **Step 6: Commit**

```bash
git add internal/ccusage/ccusage.go internal/ccusage/ccusage_test.go internal/ccusage/testdata/daily.json internal/ccusage/testdata/session.json
git commit -m "feat: invoke and parse ccusage daily/session reports"
```

---

## Task 8: Export rows to CSV and JSON

The row types here are the tool's output schema. `analyze` (Task 9) builds slices of these.

**Files:**
- Create: `internal/export/export.go`
- Test: `internal/export/export_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/export/export_test.go`:
```go
package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSessionsCSVAndJSON(t *testing.T) {
	dir := t.TempDir()
	rows := []SessionRow{
		{Repo: "/Users/me/dev/Iakuvo", ProjectDir: "-Users-me-dev-Iakuvo",
			Model: "claude-opus-4-7", InputTokens: 60, OutputTokens: 120,
			CacheCreationTokens: 20, CacheReadTokens: 25, Cost: 1.0},
	}
	if err := WriteSessions(dir, rows); err != nil {
		t.Fatalf("WriteSessions: %v", err)
	}

	csvData, err := os.ReadFile(filepath.Join(dir, "sessions.csv"))
	if err != nil {
		t.Fatal(err)
	}
	header := "repo,project_dir,model,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,cost"
	if !strings.HasPrefix(string(csvData), header) {
		t.Errorf("sessions.csv header = %q, want prefix %q", csvData, header)
	}
	if !strings.Contains(string(csvData), "claude-opus-4-7") {
		t.Errorf("sessions.csv missing data row: %s", csvData)
	}

	jsonData, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back []SessionRow
	if err := json.Unmarshal(jsonData, &back); err != nil {
		t.Fatalf("sessions.json not valid JSON: %v", err)
	}
	if len(back) != 1 || back[0].Model != "claude-opus-4-7" {
		t.Errorf("sessions.json round-trip = %+v", back)
	}
}

func TestWriteDailyCSVAndJSON(t *testing.T) {
	dir := t.TempDir()
	rows := []DailyRow{
		{Date: "2026-05-15", Repo: "/Users/me/dev/Iakuvo",
			Model: "claude-opus-4-7", InputTokens: 60, OutputTokens: 120,
			CacheCreationTokens: 20, CacheReadTokens: 25, Cost: 1.0},
	}
	if err := WriteDaily(dir, rows); err != nil {
		t.Fatalf("WriteDaily: %v", err)
	}
	csvData, err := os.ReadFile(filepath.Join(dir, "daily.csv"))
	if err != nil {
		t.Fatal(err)
	}
	header := "date,repo,model,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,cost"
	if !strings.HasPrefix(string(csvData), header) {
		t.Errorf("daily.csv header = %q, want prefix %q", csvData, header)
	}
	if _, err := os.Stat(filepath.Join(dir, "daily.json")); err != nil {
		t.Errorf("daily.json missing: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/export/ -v`
Expected: FAIL — `undefined: SessionRow`.

- [ ] **Step 3: Implement the row types and writers**

Create `internal/export/export.go`:
```go
// Package export writes analysis results as CSV and JSON files.
package export

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// SessionRow is one row of sessions.{csv,json}: one (project-dir, repo, model)
// triple. ccusage cannot break usage down per transcript UUID, so ProjectDir
// (the ccusage project key) is the finest identifier available.
type SessionRow struct {
	Repo                string  `json:"repo"`
	ProjectDir          string  `json:"project_dir"`
	Model               string  `json:"model"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	Cost                float64 `json:"cost"`
}

// DailyRow is one row of daily.{csv,json}: one (date, repo, model) triple.
type DailyRow struct {
	Date                string  `json:"date"`
	Repo                string  `json:"repo"`
	Model               string  `json:"model"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	Cost                float64 `json:"cost"`
}

// WriteSessions writes sessions.csv and sessions.json into dir.
func WriteSessions(dir string, rows []SessionRow) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	header := []string{"repo", "project_dir", "model", "input_tokens",
		"output_tokens", "cache_creation_tokens", "cache_read_tokens", "cost"}
	recs := make([][]string, len(rows))
	for i, r := range rows {
		recs[i] = []string{r.Repo, r.ProjectDir, r.Model,
			itoa(r.InputTokens), itoa(r.OutputTokens),
			itoa(r.CacheCreationTokens), itoa(r.CacheReadTokens),
			ftoa(r.Cost)}
	}
	if err := writeCSV(filepath.Join(dir, "sessions.csv"), header, recs); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "sessions.json"), rows)
}

// WriteDaily writes daily.csv and daily.json into dir.
func WriteDaily(dir string, rows []DailyRow) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	header := []string{"date", "repo", "model", "input_tokens",
		"output_tokens", "cache_creation_tokens", "cache_read_tokens", "cost"}
	recs := make([][]string, len(rows))
	for i, r := range rows {
		recs[i] = []string{r.Date, r.Repo, r.Model,
			itoa(r.InputTokens), itoa(r.OutputTokens),
			itoa(r.CacheCreationTokens), itoa(r.CacheReadTokens),
			ftoa(r.Cost)}
	}
	if err := writeCSV(filepath.Join(dir, "daily.csv"), header, recs); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "daily.json"), rows)
}

func writeCSV(path string, header []string, records [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := w.WriteAll(records); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func itoa(n int64) string     { return strconv.FormatInt(n, 10) }
func ftoa(f float64) string   { return strconv.FormatFloat(f, 'f', -1, 64) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/export/ -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/export/export.go internal/export/export_test.go
git commit -m "feat: write sessions and daily rows to CSV and JSON"
```

---

## Task 9: Analyze orchestrator — join ccusage output to repo groups

**Files:**
- Create: `internal/analyze/analyze.go`
- Test: `internal/analyze/analyze_test.go`

This task wires repo resolution + ccusage + export together. ccusage is injected as an interface so the test feeds canned reports (no subprocess).

- [ ] **Step 1: Write the failing test**

Create `internal/analyze/analyze_test.go`:
```go
package analyze

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
	"github.com/petertrost/claude-code-token-metrics/internal/export"
)

// fakeCCUsage returns canned reports, ignoring the store path.
type fakeCCUsage struct {
	daily   *ccusage.DailyReport
	session *ccusage.SessionReport
}

func (f fakeCCUsage) Daily(string) (*ccusage.DailyReport, error)     { return f.daily, nil }
func (f fakeCCUsage) Session(string) (*ccusage.SessionReport, error) { return f.session, nil }

func TestRunJoinsCCUsageToRepos(t *testing.T) {
	store := t.TempDir()
	projects := filepath.Join(store, "projects")

	// One project dir whose transcript cwd is a live git repo.
	repoDir := t.TempDir() // acts as the resolved repo
	gitInit(t, repoDir)
	projDir := filepath.Join(projects, "-proj-a")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLine(t, filepath.Join(projDir, "s.jsonl"),
		`{"type":"assistant","cwd":"`+repoDir+`"}`)

	fake := fakeCCUsage{
		daily: &ccusage.DailyReport{Projects: map[string][]ccusage.DailyRow{
			"-proj-a": {{
				Date: "2026-05-15",
				ModelBreakdowns: []ccusage.ModelBreakdown{
					{ModelName: "claude-opus-4-7", InputTokens: 10,
						OutputTokens: 20, CacheCreationTokens: 1,
						CacheReadTokens: 2, Cost: 0.5},
				},
			}},
		}},
		session: &ccusage.SessionReport{Sessions: []ccusage.SessionEntry{{
			SessionID: "-proj-a",
			ModelBreakdowns: []ccusage.ModelBreakdown{
				{ModelName: "claude-opus-4-7", InputTokens: 10,
					OutputTokens: 20, CacheCreationTokens: 1,
					CacheReadTokens: 2, Cost: 0.5},
			},
		}}},
	}

	out := t.TempDir()
	res, err := Run(Config{
		StoreRoot:     store,
		SquadStatePath: filepath.Join(t.TempDir(), "no-squad.json"),
		OutDir:        out,
		CCUsage:       fake,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(res.Daily) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(res.Daily))
	}
	wantRepo := evalSym(t, repoDir)
	gotRepo := evalSym(t, res.Daily[0].Repo)
	if gotRepo != wantRepo {
		t.Errorf("daily row repo = %q, want %q", res.Daily[0].Repo, wantRepo)
	}
	if res.Daily[0].Model != "claude-opus-4-7" || res.Daily[0].InputTokens != 10 {
		t.Errorf("daily row = %+v", res.Daily[0])
	}
	if len(res.Sessions) != 1 {
		t.Errorf("session rows = %d, want 1", len(res.Sessions))
	}
	// Output files written.
	for _, name := range []string{"daily.csv", "daily.json", "sessions.csv", "sessions.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing output %s: %v", name, err)
		}
	}
}

func TestRunUnknownRepoBucket(t *testing.T) {
	store := t.TempDir()
	projects := filepath.Join(store, "projects")
	projDir := filepath.Join(projects, "-proj-x")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Transcript with no cwd -> unresolvable.
	writeLine(t, filepath.Join(projDir, "s.jsonl"), `{"type":"system"}`)

	fake := fakeCCUsage{
		daily: &ccusage.DailyReport{Projects: map[string][]ccusage.DailyRow{
			"-proj-x": {{Date: "2026-05-15", ModelBreakdowns: []ccusage.ModelBreakdown{
				{ModelName: "claude-haiku-4-5-20251001", InputTokens: 1},
			}}},
		}},
		session: &ccusage.SessionReport{},
	}
	res, err := Run(Config{
		StoreRoot:      store,
		SquadStatePath: filepath.Join(t.TempDir(), "none.json"),
		OutDir:         t.TempDir(),
		CCUsage:        fake,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Daily) != 1 || res.Daily[0].Repo != "unknown" {
		t.Errorf("expected one row bucketed as unknown, got %+v", res.Daily)
	}
}

// --- helpers ---

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t.t"}, {"config", "user.name", "t"},
	} {
		runGit(t, dir, args...)
	}
}

func evalSym(t *testing.T, p string) string {
	t.Helper()
	r, err := filepathEvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func writeLine(t *testing.T, path, line string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

var _ = sort.Strings // keep sort import available for future ordering tests
```

> Note: `runGit` and `filepathEvalSymlinks` are tiny wrappers — add them to the test file:
```go
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := execCommand("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
```
Add imports `os/exec` (aliased call `execCommand := exec.Command` is unnecessary — just use `exec.Command` directly and import `"os/exec"`) and `"path/filepath"` (use `filepath.EvalSymlinks` directly instead of `filepathEvalSymlinks`). Rewrite the two helpers to call `exec.Command` and `filepath.EvalSymlinks` directly; the alias names above were placeholders. Final test imports: `os`, `os/exec`, `path/filepath`, `sort`, `testing`, plus the two internal packages.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/analyze/ -v`
Expected: FAIL — `undefined: Run` / `undefined: Config`.

- [ ] **Step 3: Implement the analyze orchestrator**

Create `internal/analyze/analyze.go`:
```go
// Package analyze joins ccusage usage data to resolved git repositories.
package analyze

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
	"github.com/petertrost/claude-code-token-metrics/internal/export"
	"github.com/petertrost/claude-code-token-metrics/internal/repo"
)

// CCUsage is the subset of ccusage the analyzer needs. ccusage.Runner
// satisfies it; tests pass a fake.
type CCUsage interface {
	Daily(storeRoot string) (*ccusage.DailyReport, error)
	Session(storeRoot string) (*ccusage.SessionReport, error)
}

// Config holds the inputs for one analyze run.
type Config struct {
	StoreRoot      string  // snapshot store root (contains projects/)
	SquadStatePath string  // path to claude-squad state.json
	OutDir         string  // directory to write the four output files
	CCUsage        CCUsage // ccusage implementation
}

// Result is the in-memory outcome of a run, also written to OutDir.
type Result struct {
	Sessions []export.SessionRow
	Daily    []export.DailyRow
}

// Run resolves repos for every project dir in the store, calls ccusage,
// joins the two, writes the four output files, and returns the rows.
func Run(cfg Config) (*Result, error) {
	// 1. Build a project-dir -> resolved-repo map.
	squadIdx, err := repo.LoadSquadState(cfg.SquadStatePath)
	if err != nil {
		return nil, err
	}
	resolver := repo.NewResolver(squadIdx)
	repoByProject := resolveAllProjects(filepath.Join(cfg.StoreRoot, "projects"), resolver)

	// 2. Run ccusage.
	daily, err := cfg.CCUsage.Daily(cfg.StoreRoot)
	if err != nil {
		return nil, err
	}
	session, err := cfg.CCUsage.Session(cfg.StoreRoot)
	if err != nil {
		return nil, err
	}

	// 3. Join and roll up.
	res := &Result{
		Daily:    buildDailyRows(daily, repoByProject),
		Sessions: buildSessionRows(session, repoByProject),
	}

	// 4. Write outputs.
	if err := export.WriteSessions(cfg.OutDir, res.Sessions); err != nil {
		return nil, err
	}
	if err := export.WriteDaily(cfg.OutDir, res.Daily); err != nil {
		return nil, err
	}
	return res, nil
}

// resolveAllProjects resolves every project directory under projectsRoot.
// A project dir with no transcripts, or one that resolves to nothing, maps
// to repo.UnknownRepo.
func resolveAllProjects(projectsRoot string, resolver *repo.Resolver) map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return out // store may not exist yet; callers treat all as unknown
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projDir := e.Name()
		out[projDir] = repo.UnknownRepo
		// Resolve using the first transcript that yields a real repo.
		files, _ := os.ReadDir(filepath.Join(projectsRoot, projDir))
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".jsonl" {
				continue
			}
			tp := filepath.Join(projectsRoot, projDir, f.Name())
			if r := resolver.Resolve(projDir, tp); r != repo.UnknownRepo {
				out[projDir] = r
				break
			}
		}
	}
	return out
}

// repoFor returns the resolved repo for a ccusage project key, defaulting to
// UnknownRepo for project keys not seen in the store.
func repoFor(repoByProject map[string]string, projectKey string) string {
	if r, ok := repoByProject[projectKey]; ok {
		return r
	}
	return repo.UnknownRepo
}

// buildDailyRows flattens ccusage daily output to (date, repo, model) rows.
// ccusage already separates by project and date; we map project->repo and
// fan out the model breakdown. Two project dirs resolving to the same repo
// are summed into one row per (date, repo, model).
func buildDailyRows(rep *ccusage.DailyReport, repoByProject map[string]string) []export.DailyRow {
	type key struct{ date, repo, model string }
	acc := map[key]*export.DailyRow{}
	for projectKey, days := range rep.Projects {
		r := repoFor(repoByProject, projectKey)
		for _, d := range days {
			for _, mb := range d.ModelBreakdowns {
				k := key{d.Date, r, mb.ModelName}
				row := acc[k]
				if row == nil {
					row = &export.DailyRow{Date: d.Date, Repo: r, Model: mb.ModelName}
					acc[k] = row
				}
				row.InputTokens += mb.InputTokens
				row.OutputTokens += mb.OutputTokens
				row.CacheCreationTokens += mb.CacheCreationTokens
				row.CacheReadTokens += mb.CacheReadTokens
				row.Cost += mb.Cost
			}
		}
	}
	out := make([]export.DailyRow, 0, len(acc))
	for _, row := range acc {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// buildSessionRows flattens ccusage session output to (project-dir, repo,
// model) rows. ccusage keys sessions by project directory, so ProjectDir is
// the ccusage sessionId.
func buildSessionRows(rep *ccusage.SessionReport, repoByProject map[string]string) []export.SessionRow {
	out := []export.SessionRow{}
	for _, s := range rep.Sessions {
		r := repoFor(repoByProject, s.SessionID)
		for _, mb := range s.ModelBreakdowns {
			out = append(out, export.SessionRow{
				Repo:                r,
				ProjectDir:          s.SessionID,
				Model:               mb.ModelName,
				InputTokens:         mb.InputTokens,
				OutputTokens:        mb.OutputTokens,
				CacheCreationTokens: mb.CacheCreationTokens,
				CacheReadTokens:     mb.CacheReadTokens,
				Cost:                mb.Cost,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		if out[i].ProjectDir != out[j].ProjectDir {
			return out[i].ProjectDir < out[j].ProjectDir
		}
		return out[i].Model < out[j].Model
	})
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/analyze/ -v`
Expected: PASS — both tests.

- [ ] **Step 5: Wire `analyze` into main**

In `main.go`, replace the `case "analyze":` body:
```go
	case "analyze":
		if err := runAnalyze(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "analyze:", err)
			os.Exit(1)
		}
```
Add to `main.go`:
```go
func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	outDir := fs.String("out", "out", "directory for the CSV/JSON output files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := analyze.Run(analyze.Config{
		StoreRoot:      paths.StoreRoot(),
		SquadStatePath: paths.SquadStateFile(),
		OutDir:         *outDir,
		CCUsage:        ccusage.DefaultRunner(),
	})
	if err != nil {
		return err
	}
	fmt.Printf("analyze: wrote %d session rows and %d daily rows to %s/\n",
		len(res.Sessions), len(res.Daily), *outDir)
	return nil
}
```
Update the `main.go` import block to add `"flag"`, `internal/analyze`, and `internal/ccusage`:
```go
import (
	"flag"
	"fmt"
	"os"

	"github.com/petertrost/claude-code-token-metrics/internal/analyze"
	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
	"github.com/petertrost/claude-code-token-metrics/internal/paths"
	"github.com/petertrost/claude-code-token-metrics/internal/snapshot"
)
```

- [ ] **Step 6: Verify build and a real analyze run**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go build ./... && go run . sweep && go run . analyze --out /tmp/cctm-out && head -3 /tmp/cctm-out/daily.csv
```
Expected: build succeeds; sweep + analyze run; `daily.csv` has the header and real rows. (Requires `npx ccusage` to be reachable — first run downloads it.)

- [ ] **Step 7: Commit**

```bash
git add internal/analyze/analyze.go internal/analyze/analyze_test.go main.go
git commit -m "feat: analyze command joining ccusage output to repo groups"
```

---

## Task 10: The SessionEnd hook script

**Files:**
- Create: `hooks/snapshot-transcript.sh`
- Test: `hooks/snapshot-transcript_test.sh` (a plain bash test, run manually + in CI)

- [ ] **Step 1: Write the hook script**

Create `hooks/snapshot-transcript.sh`:
```bash
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
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x /Users/petertrost/dev/claude-code-token-metrics/hooks/snapshot-transcript.sh`

- [ ] **Step 3: Write a bash test for the hook**

Create `hooks/snapshot-transcript_test.sh`:
```bash
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
```

- [ ] **Step 4: Run the hook test**

Run:
```bash
chmod +x /Users/petertrost/dev/claude-code-token-metrics/hooks/snapshot-transcript_test.sh
/Users/petertrost/dev/claude-code-token-metrics/hooks/snapshot-transcript_test.sh
```
Expected: `PASS: snapshot-transcript.sh`.

- [ ] **Step 5: Commit**

```bash
git add hooks/snapshot-transcript.sh hooks/snapshot-transcript_test.sh
git commit -m "feat: SessionEnd hook script to snapshot transcripts"
```

---

## Task 11: Setup — merge the hook into settings.json

The critical requirement: **merge, never clobber.** Existing `PreToolUse` / `Stop` / `Notification` hooks must survive, and re-running `setup` must not add a duplicate `SessionEnd` entry.

**Files:**
- Create: `internal/setup/settings.go`
- Test: `internal/setup/settings_test.go`
- Create: `internal/setup/testdata/settings-with-hooks.json`

- [ ] **Step 1: Write the fixture**

Create `internal/setup/testdata/settings-with-hooks.json` (a settings file that already has unrelated hooks):
```json
{
  "model": "opus",
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [ { "type": "command", "command": "echo pre" } ] }
    ]
  }
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/setup/settings_test.go`:
```go
package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// loadHooks parses settings JSON and returns the hooks object.
func loadHooks(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings not valid JSON: %v", err)
	}
	h, _ := m["hooks"].(map[string]any)
	return h
}

func TestMergeSessionEndHookPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	src, _ := os.ReadFile("testdata/settings-with-hooks.json")
	if err := os.WriteFile(settings, src, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MergeSessionEndHook(settings, "/abs/hooks/snapshot-transcript.sh"); err != nil {
		t.Fatalf("MergeSessionEndHook: %v", err)
	}

	hooks := loadHooks(t, settings)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse hook was clobbered")
	}
	se, ok := hooks["SessionEnd"].([]any)
	if !ok || len(se) != 1 {
		t.Fatalf("SessionEnd = %v, want one entry", hooks["SessionEnd"])
	}
}

func TestMergeSessionEndHookIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	src, _ := os.ReadFile("testdata/settings-with-hooks.json")
	if err := os.WriteFile(settings, src, 0o644); err != nil {
		t.Fatal(err)
	}
	script := "/abs/hooks/snapshot-transcript.sh"
	if err := MergeSessionEndHook(settings, script); err != nil {
		t.Fatal(err)
	}
	if err := MergeSessionEndHook(settings, script); err != nil {
		t.Fatal(err)
	}
	hooks := loadHooks(t, settings)
	se := hooks["SessionEnd"].([]any)
	if len(se) != 1 {
		t.Errorf("SessionEnd has %d entries after two merges, want 1", len(se))
	}
}

func TestMergeSessionEndHookCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json") // does not exist yet
	if err := MergeSessionEndHook(settings, "/abs/hook.sh"); err != nil {
		t.Fatalf("MergeSessionEndHook on missing file: %v", err)
	}
	hooks := loadHooks(t, settings)
	if _, ok := hooks["SessionEnd"]; !ok {
		t.Error("SessionEnd not added to freshly created settings file")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/setup/ -run TestMerge -v`
Expected: FAIL — `undefined: MergeSessionEndHook`.

- [ ] **Step 4: Implement the merge**

Create `internal/setup/settings.go`:
```go
// Package setup installs the SessionEnd hook and the launchd sweep job.
package setup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// MergeSessionEndHook adds a SessionEnd hook running scriptPath into the
// Claude Code settings file at settingsPath. It preserves every other key and
// every other hook event, and is idempotent: a SessionEnd entry already
// pointing at scriptPath is not duplicated. A missing settings file is
// created with just the hooks object.
func MergeSessionEndHook(settingsPath, scriptPath string) error {
	settings := map[string]any{}

	data, err := os.ReadFile(settingsPath)
	switch {
	case err == nil:
		if len(data) > 0 {
			if err := json.Unmarshal(data, &settings); err != nil {
				return err
			}
		}
	case errors.Is(err, fs.ErrNotExist):
		// settings will be created fresh below.
	default:
		return err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	// SessionEnd is an array of matcher-groups; each group has a hooks array.
	sessionEnd, _ := hooks["SessionEnd"].([]any)

	// Idempotency: bail if any entry already runs our script.
	if sessionEndContainsScript(sessionEnd, scriptPath) {
		return nil
	}

	newGroup := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": scriptPath},
		},
	}
	hooks["SessionEnd"] = append(sessionEnd, newGroup)
	settings["hooks"] = hooks

	return writeSettings(settingsPath, settings)
}

// sessionEndContainsScript reports whether any SessionEnd group already runs
// command == scriptPath.
func sessionEndContainsScript(sessionEnd []any, scriptPath string) bool {
	for _, g := range sessionEnd {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); cmd == scriptPath {
				return true
			}
		}
	}
	return false
}

// writeSettings writes settings as indented JSON, creating parent dirs.
func writeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/setup/ -run TestMerge -v`
Expected: PASS — all three tests.

- [ ] **Step 6: Commit**

```bash
git add internal/setup/settings.go internal/setup/settings_test.go internal/setup/testdata/settings-with-hooks.json
git commit -m "feat: merge SessionEnd hook into settings.json without clobbering"
```

---

## Task 12: Setup — launchd sweep job + hook script install + wire into main

Renders a launchd plist that runs `claude-code-token-metrics sweep` every 4 hours, copies the hook script into the store, and wires `setup` into `main`.

**Files:**
- Create: `internal/setup/launchd.go`
- Test: `internal/setup/launchd_test.go`
- Modify: `internal/setup/settings.go` — add `InstallHookScript` + `Run`
- Modify: `main.go`
- Modify: hook script needs an install location: `~/.claude-token-metrics/hooks/snapshot-transcript.sh`

- [ ] **Step 1: Write the failing test for the plist**

Create `internal/setup/launchd_test.go`:
```go
package setup

import (
	"strings"
	"testing"
)

func TestRenderSweepPlist(t *testing.T) {
	plist := RenderSweepPlist("/usr/local/bin/claude-code-token-metrics")

	for _, want := range []string{
		"<?xml",
		"com.petertrost.claude-code-token-metrics.sweep", // Label
		"/usr/local/bin/claude-code-token-metrics",       // binary path
		"<string>sweep</string>",                         // the argument
		"<key>StartInterval</key>",
		"<integer>14400</integer>",                       // 4 hours
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q\n---\n%s", want, plist)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/setup/ -run TestRenderSweepPlist -v`
Expected: FAIL — `undefined: RenderSweepPlist`.

- [ ] **Step 3: Implement the plist renderer and installer**

Create `internal/setup/launchd.go`:
```go
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SweepLabel is the launchd job label; also the plist filename stem.
const SweepLabel = "com.petertrost.claude-code-token-metrics.sweep"

// sweepIntervalSeconds is the StartInterval for the sweep job (4 hours).
const sweepIntervalSeconds = 14400

// RenderSweepPlist returns a launchd plist that runs `<binPath> sweep` every
// sweepIntervalSeconds.
func RenderSweepPlist(binPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>sweep</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, SweepLabel, binPath, sweepIntervalSeconds)
}

// InstallSweepJob writes the plist into ~/Library/LaunchAgents and (re)loads
// it with launchctl. homeDir is passed in so tests can use a temp dir.
func InstallSweepJob(homeDir, binPath string) (string, error) {
	agentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return "", err
	}
	plistPath := filepath.Join(agentsDir, SweepLabel+".plist")
	if err := os.WriteFile(plistPath, []byte(RenderSweepPlist(binPath)), 0o644); err != nil {
		return "", err
	}
	// Reload: unload first (ignore error if not loaded), then load.
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		// Non-fatal: the plist is on disk and will load at next login.
		return plistPath, fmt.Errorf("plist written to %s but launchctl load failed: %w", plistPath, err)
	}
	return plistPath, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/setup/ -run TestRenderSweepPlist -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test for hook-script install**

Append to `internal/setup/settings_test.go`:
```go
func TestInstallHookScript(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "snapshot-transcript.sh")
	if err := os.WriteFile(src, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	destDir := t.TempDir()
	dest := filepath.Join(destDir, "hooks", "snapshot-transcript.sh")

	if err := InstallHookScript(src, dest); err != nil {
		t.Fatalf("InstallHookScript: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("hook script not installed: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("installed hook script is not executable")
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/setup/ -run TestInstallHookScript -v`
Expected: FAIL — `undefined: InstallHookScript`.

- [ ] **Step 7: Implement InstallHookScript**

Append to `internal/setup/settings.go`:
```go
// InstallHookScript copies the hook script from src to dest, creating dest's
// parent directory and marking the result executable.
func InstallHookScript(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o755)
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /Users/petertrost/dev/claude-code-token-metrics && go test ./internal/setup/ -run TestInstallHookScript -v`
Expected: PASS.

- [ ] **Step 9: Add the setup orchestrator**

Append to `internal/setup/settings.go`:
```go
// Config holds the inputs for a setup run.
type Config struct {
	SettingsPath   string // ~/.claude/settings.json
	HookScriptSrc  string // hooks/snapshot-transcript.sh in the repo
	HookScriptDest string // ~/.claude-token-metrics/hooks/snapshot-transcript.sh
	HomeDir        string // for the launchd agents dir
	BinPath        string // absolute path to the installed binary
}

// Run installs the hook script, merges the SessionEnd hook, and installs the
// launchd sweep job. It returns a human-readable summary.
func Run(cfg Config) (string, error) {
	if err := InstallHookScript(cfg.HookScriptSrc, cfg.HookScriptDest); err != nil {
		return "", err
	}
	if err := MergeSessionEndHook(cfg.SettingsPath, cfg.HookScriptDest); err != nil {
		return "", err
	}
	plistPath, err := InstallSweepJob(cfg.HomeDir, cfg.BinPath)
	summary := "setup complete:\n" +
		"  hook script: " + cfg.HookScriptDest + "\n" +
		"  SessionEnd hook merged into: " + cfg.SettingsPath + "\n" +
		"  launchd job: " + plistPath
	if err != nil {
		// launchctl load failed but everything is on disk.
		return summary, err
	}
	return summary, nil
}
```

- [ ] **Step 10: Wire `setup` into main**

In `main.go`, replace the `case "setup":` body:
```go
	case "setup":
		if err := runSetup(); err != nil {
			fmt.Fprintln(os.Stderr, "setup:", err)
			os.Exit(1)
		}
```
Add to `main.go`:
```go
func runSetup() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// The hook script ships next to the repo; locate it relative to the binary
	// at build time is not reliable, so require it via the CCTM_REPO env var
	// when running from source, else assume it sits beside the binary.
	hookSrc := os.Getenv("CCTM_HOOK_SRC")
	if hookSrc == "" {
		hookSrc = filepath.Join(filepath.Dir(exe), "hooks", "snapshot-transcript.sh")
	}
	summary, err := setup.Run(setup.Config{
		SettingsPath:   paths.ClaudeSettingsFile(),
		HookScriptSrc:  hookSrc,
		HookScriptDest: filepath.Join(paths.HomeDir(), ".claude-token-metrics", "hooks", "snapshot-transcript.sh"),
		HomeDir:        paths.HomeDir(),
		BinPath:        exe,
	})
	if summary != "" {
		fmt.Println(summary)
	}
	return err
}
```
Update the `main.go` import block to add `"path/filepath"` and `internal/setup`:
```go
import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/petertrost/claude-code-token-metrics/internal/analyze"
	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
	"github.com/petertrost/claude-code-token-metrics/internal/paths"
	"github.com/petertrost/claude-code-token-metrics/internal/setup"
	"github.com/petertrost/claude-code-token-metrics/internal/snapshot"
)
```

- [ ] **Step 11: Verify build and the full package test**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go build ./... && go test ./... -v 2>&1 | tail -20
```
Expected: build succeeds; every package's tests PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/setup/launchd.go internal/setup/launchd_test.go internal/setup/settings.go internal/setup/settings_test.go main.go
git commit -m "feat: setup command installs hook script, SessionEnd hook, and launchd sweep job"
```

---

## Task 13: README, repo hygiene, and end-to-end smoke check

**Files:**
- Create: `README.md`
- Verify: `.gitignore` already contains `snapshots/` and `out/` (it does — confirmed at plan time).
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the README**

Create `README.md`:
```markdown
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
```

- [ ] **Step 2: Add a CI workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: ci
on:
  push:
  pull_request:
jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: go vet
        run: go vet ./...
      - name: go test
        run: go test ./...
      - name: hook script test
        run: bash hooks/snapshot-transcript_test.sh
```

- [ ] **Step 3: Run the full verification suite**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go vet ./... && go test ./... && bash hooks/snapshot-transcript_test.sh
```
Expected: `go vet` clean; all Go tests PASS; `PASS: snapshot-transcript.sh`.

- [ ] **Step 4: End-to-end smoke check**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics
go build -o /tmp/cctm . \
  && /tmp/cctm sweep \
  && /tmp/cctm analyze --out /tmp/cctm-smoke \
  && echo "--- daily.csv ---" && head -5 /tmp/cctm-smoke/daily.csv \
  && echo "--- sessions.csv ---" && head -5 /tmp/cctm-smoke/sessions.csv
```
Expected: build succeeds; sweep populates `~/.claude-token-metrics/snapshots/projects/`; analyze writes four files into `/tmp/cctm-smoke`; both CSVs show a header and real data rows. Confirm at least one row has a real repo path (not all `unknown`).

- [ ] **Step 5: Confirm git status is clean of snapshots**

Run:
```bash
cd /Users/petertrost/dev/claude-code-token-metrics && git status --porcelain
```
Expected: only `README.md` and `.github/workflows/ci.yml` show as new — **no `snapshots/` or `out/` entries**. If snapshots appear, stop and fix `.gitignore` before committing.

- [ ] **Step 6: Commit**

```bash
git add README.md .github/workflows/ci.yml
git commit -m "docs: add README and CI workflow"
```

---

## Self-review against the design

**Spec coverage:**
- Capture via SessionEnd hook → Task 10 (script), Task 11–12 (install).
- Capture via periodic sweep → Task 6 (`sweep`), Task 12 (launchd job).
- Snapshot store layout → Task 6; **corrected** to include `projects/` subdir (Background fact 1) so ccusage works.
- Repo grouping & resolution heuristic (live repo / live worktree / dead worktree / unknown) → Tasks 2–5.
- Analysis via ccusage → Task 7; `analyze` join + rollup → Task 9.
- Outputs `sessions.{csv,json}` + `daily.{csv,json}` → Tasks 8–9; **`sessions` grain redefined** to `(project_dir, repo, model)` because ccusage cannot break down per transcript UUID (Background fact 3).
- Error handling: ccusage not installed → `ccusage.ErrNotInstalled` (Task 7); malformed transcript line skipped → `FirstCwd` skips bad lines (Task 3); unresolved repo → `unknown` bucket (Tasks 5, 9).
- CLI surface `setup` / `sweep` / `analyze [--out]` → Tasks 1, 6, 9, 12.
- Merge-not-clobber settings, idempotent → Task 11.
- Public repo ships only code + hook + setup; snapshots gitignored → Task 13 (README + `.gitignore` check).
- Testing of resolution heuristic, capture, ccusage-mocked analysis, setup → Tasks 2–12 each TDD.

**Resolved open choices from the design:**
- Language/packaging → Go 1.26, single static binary, stdlib only (user-confirmed).
- launchd interval/label → `StartInterval` 14400s, label `com.petertrost.claude-code-token-metrics.sweep` (Task 12).

**Deviations from the design, all justified by verified facts (see Background section):**
1. Store layout gains a `projects/` level (ccusage needs `CLAUDE_CONFIG_DIR` + `projects/`).
2. Repo resolution reads the *first non-null* `cwd`, not line 1 (`cwd` is not on every line).
3. `sessions.csv` grain is `(project_dir, repo, model)`, not per-transcript-UUID (ccusage limitation).

The design's Step-3 normalization (`NormalizeWorktreeBranch`, Task 4) is built and unit-tested but not yet wired into `Resolver.Resolve`, which currently relies on the squad-state *exact* match for dead worktrees. This is intentional: exact match is precise; the normalized branch is a lower-confidence fallback. If real data shows dead worktrees missing from squad state, a follow-up task can add a branch-based fuzzy match using `NormalizeWorktreeBranch`. Flagged here so the executing engineer knows it is deliberate, not an omission.
