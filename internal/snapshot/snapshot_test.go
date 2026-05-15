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
