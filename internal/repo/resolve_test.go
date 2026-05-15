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
	resolver := NewResolver(map[string]string{})
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
	resolver := NewResolver(map[string]string{})
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
