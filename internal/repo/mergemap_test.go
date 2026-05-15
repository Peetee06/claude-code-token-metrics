package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeSquadStateIntoRepoMap(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	mapPath := filepath.Join(dir, "repo-map.json")

	// A pre-existing repo-map entry that must survive the merge.
	if err := WriteRepoMap(mapPath, map[string]string{
		"/old/worktree": "/old/repo",
	}); err != nil {
		t.Fatal(err)
	}
	// A claude-squad state with one live instance.
	state := `{"instances":[{"worktree":{
		"repo_path":"/Users/me/dev/Iakuvo",
		"worktree_path":"/Users/me/.claude-squad/worktrees/feat/x_abc"}}]}`
	if err := os.WriteFile(statePath, []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeSquadStateIntoRepoMap(statePath, mapPath)
	if err != nil {
		t.Fatalf("MergeSquadStateIntoRepoMap: %v", err)
	}
	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}

	got, err := LoadRepoMap(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if got["/Users/me/.claude-squad/worktrees/feat/x_abc"] != "/Users/me/dev/Iakuvo" {
		t.Errorf("squad worktree not merged: %v", got)
	}
	if got["/old/worktree"] != "/old/repo" {
		t.Errorf("pre-existing entry was clobbered: %v", got)
	}
}

// An existing repo-map entry is authoritative — it is NOT overwritten by
// claude-squad state (the hook captured it deliberately).
func TestMergeSquadStateDoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	mapPath := filepath.Join(dir, "repo-map.json")

	cwd := "/Users/me/.claude-squad/worktrees/feat/x_abc"
	if err := WriteRepoMap(mapPath, map[string]string{cwd: "/repo/from/hook"}); err != nil {
		t.Fatal(err)
	}
	state := `{"instances":[{"worktree":{
		"repo_path":"/repo/from/state","worktree_path":"` + cwd + `"}}]}`
	if err := os.WriteFile(statePath, []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeSquadStateIntoRepoMap(statePath, mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0 (entry already present)", added)
	}
	got, _ := LoadRepoMap(mapPath)
	if got[cwd] != "/repo/from/hook" {
		t.Errorf("existing entry = %q, want /repo/from/hook (state must not overwrite)", got[cwd])
	}
}

// Missing claude-squad state is not an error — the machine may not run Squad.
func TestMergeSquadStateMissingStateFile(t *testing.T) {
	dir := t.TempDir()
	mapPath := filepath.Join(dir, "repo-map.json")
	added, err := MergeSquadStateIntoRepoMap(filepath.Join(dir, "nope.json"), mapPath)
	if err != nil {
		t.Fatalf("missing state file should not error: %v", err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
}
