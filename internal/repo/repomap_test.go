package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRepoMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo-map.json")
	content := `{
  "/Users/me/.claude-squad/worktrees/feat/x_abc": "/Users/me/dev/Iakuvo",
  "/Users/me/dev/other": "/Users/me/dev/other"
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadRepoMap(path)
	if err != nil {
		t.Fatalf("LoadRepoMap: %v", err)
	}
	if idx["/Users/me/.claude-squad/worktrees/feat/x_abc"] != "/Users/me/dev/Iakuvo" {
		t.Errorf("worktree lookup = %q", idx["/Users/me/.claude-squad/worktrees/feat/x_abc"])
	}
	if len(idx) != 2 {
		t.Errorf("index size = %d, want 2", len(idx))
	}
}

func TestLoadRepoMapMissingFile(t *testing.T) {
	idx, err := LoadRepoMap(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if idx == nil {
		t.Error("missing file should yield an empty (non-nil) index")
	}
}
