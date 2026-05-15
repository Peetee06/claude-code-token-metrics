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
