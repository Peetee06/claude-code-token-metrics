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
