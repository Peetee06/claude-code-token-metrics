package ccusage

import (
	"os"
	"testing"
)

func TestParseDaily(t *testing.T) {
	data, err := os.ReadFile("testdata/daily.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ParseDaily(data)
	if err != nil {
		t.Fatalf("ParseDaily: %v", err)
	}
	rows := rep.Projects["-Users-me-dev-Iakuvo"]
	if len(rows) != 1 {
		t.Fatalf("Iakuvo day count = %d, want 1", len(rows))
	}
	if rows[0].Date != "2026-05-15" {
		t.Errorf("date = %q, want 2026-05-15", rows[0].Date)
	}
	if len(rows[0].ModelBreakdowns) != 2 {
		t.Fatalf("model breakdown count = %d, want 2", len(rows[0].ModelBreakdowns))
	}
	mb := rows[0].ModelBreakdowns[0]
	if mb.ModelName != "claude-opus-4-7" || mb.InputTokens != 60 || mb.Cost != 1.0 {
		t.Errorf("first model breakdown = %+v", mb)
	}
}

func TestParseSession(t *testing.T) {
	data, err := os.ReadFile("testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ParseSession(data)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if len(rep.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(rep.Sessions))
	}
	s := rep.Sessions[0]
	if s.SessionID != "-Users-me-dev-Iakuvo" {
		t.Errorf("sessionId = %q", s.SessionID)
	}
	if len(s.ModelBreakdowns) != 2 {
		t.Errorf("model breakdown count = %d, want 2", len(s.ModelBreakdowns))
	}
}
