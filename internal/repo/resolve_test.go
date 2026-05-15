package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeTranscript writes a one-line transcript whose cwd is the given path.
func writeTranscript(t *testing.T, dir, cwd string) string {
	t.Helper()
	p := filepath.Join(dir, "t.jsonl")
	line := `{"type":"assistant","cwd":"` + cwd + `"}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// runGit runs `git -C dir args...` and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestResolveLiveGitRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.t")
	runGit(t, dir, "config", "user.name", "t")

	transcript := writeTranscript(t, dir, dir)
	resolver := NewResolver(ResolverSources{})
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
	resolver := NewResolver(ResolverSources{
		SquadState: map[string]string{
			"/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0": "/Users/me/dev/Iakuvo",
		},
	})
	got := resolver.Resolve("project-dir", "testdata/transcript-dead-worktree.jsonl")
	if got != "/Users/me/dev/Iakuvo" {
		t.Errorf("Resolve dead worktree = %q, want /Users/me/dev/Iakuvo", got)
	}
}

// The scenario the durable repo-map fixes: the worktree is deleted AND the
// instance is gone from claude-squad state.json, but the SessionStart hook
// recorded the cwd -> repo mapping while the instance was alive.
func TestResolveDeadWorktreeViaRepoMap(t *testing.T) {
	cwd := "/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0"
	resolver := NewResolver(ResolverSources{
		RepoMap:    map[string]string{cwd: "/Users/me/dev/Iakuvo"},
		SquadState: map[string]string{}, // instance already deleted from state
	})
	got := resolver.Resolve("project-dir", "testdata/transcript-dead-worktree.jsonl")
	if got != "/Users/me/dev/Iakuvo" {
		t.Errorf("Resolve via repo-map = %q, want /Users/me/dev/Iakuvo", got)
	}
}

// repo-map takes precedence over the volatile squad state when both have an
// entry for the same cwd.
func TestResolveRepoMapBeatsSquadState(t *testing.T) {
	cwd := "/Users/me/.claude-squad/worktrees/fix/archived-inspections_18afafe44922d0d0"
	resolver := NewResolver(ResolverSources{
		RepoMap:    map[string]string{cwd: "/Users/me/dev/correct"},
		SquadState: map[string]string{cwd: "/Users/me/dev/stale"},
	})
	got := resolver.Resolve("project-dir", "testdata/transcript-dead-worktree.jsonl")
	if got != "/Users/me/dev/correct" {
		t.Errorf("Resolve = %q, want repo-map value /Users/me/dev/correct", got)
	}
}

// Historical Squad sessions: worktree gone, no repo-map entry, no squad-state
// entry — the blanket SquadDefaultRepo fallback attributes them.
func TestResolveSquadDefaultFallback(t *testing.T) {
	resolver := NewResolver(ResolverSources{
		SquadDefaultRepo: "/Users/me/dev/Iakuvo",
	})
	got := resolver.Resolve("project-dir", "testdata/transcript-dead-worktree.jsonl")
	if got != "/Users/me/dev/Iakuvo" {
		t.Errorf("Resolve via squad default = %q, want /Users/me/dev/Iakuvo", got)
	}
}

// The squad-default fallback must NOT swallow a non-Squad unresolvable cwd.
func TestResolveSquadDefaultIgnoresNonSquadCwd(t *testing.T) {
	dir := t.TempDir()
	transcript := writeTranscript(t, dir, "/Users/me/dev/some-deleted-plain-dir")
	resolver := NewResolver(ResolverSources{
		SquadDefaultRepo: "/Users/me/dev/Iakuvo",
	})
	got := resolver.Resolve("project-dir", transcript)
	if got != "unknown" {
		t.Errorf("Resolve non-Squad dead cwd = %q, want unknown", got)
	}
}

func TestResolveUnknown(t *testing.T) {
	resolver := NewResolver(ResolverSources{})
	got := resolver.Resolve("project-dir", "testdata/transcript-nocwd.jsonl")
	if got != "unknown" {
		t.Errorf("Resolve with no cwd = %q, want unknown", got)
	}
}

func TestResolveLiveGitWorktreeFoldsToOriginRepo(t *testing.T) {
	mainRepo := t.TempDir()
	runGit(t, mainRepo, "init")
	runGit(t, mainRepo, "config", "user.email", "t@t.t")
	runGit(t, mainRepo, "config", "user.name", "t")
	// A worktree add requires at least one commit on the main repo.
	if err := os.WriteFile(filepath.Join(mainRepo, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, mainRepo, "add", "f.txt")
	runGit(t, mainRepo, "commit", "-m", "init")

	// Create a linked worktree in a sibling directory.
	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, mainRepo, "worktree", "add", worktree, "-b", "feature")

	// A transcript whose cwd is the linked worktree must resolve to the
	// MAIN repo root, not the worktree path.
	transcript := writeTranscript(t, t.TempDir(), worktree)
	resolver := NewResolver(ResolverSources{})
	got := resolver.Resolve("project-dir", transcript)

	wantMain, _ := filepath.EvalSymlinks(mainRepo)
	gotEval, _ := filepath.EvalSymlinks(got)
	wtEval, _ := filepath.EvalSymlinks(worktree)
	if gotEval != wantMain {
		t.Errorf("Resolve linked worktree = %q (eval %q), want main repo %q", got, gotEval, wantMain)
	}
	if gotEval == wtEval {
		t.Errorf("Resolve returned the worktree path %q instead of folding to the origin repo", got)
	}
}
