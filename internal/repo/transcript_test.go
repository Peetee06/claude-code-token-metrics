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
