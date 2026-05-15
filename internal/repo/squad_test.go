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
