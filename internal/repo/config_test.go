package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"squad_default_repo":"/Users/me/dev/Iakuvo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SquadDefaultRepo != "/Users/me/dev/Iakuvo" {
		t.Errorf("SquadDefaultRepo = %q", cfg.SquadDefaultRepo)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing config should not error, got %v", err)
	}
	if cfg.SquadDefaultRepo != "" {
		t.Errorf("missing config should yield empty SquadDefaultRepo, got %q", cfg.SquadDefaultRepo)
	}
}
