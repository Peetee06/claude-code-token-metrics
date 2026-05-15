package report

import (
	"testing"

	"github.com/petertrost/claude-code-token-metrics/internal/export"
)

func sampleRows() []export.DailyRow {
	return []export.DailyRow{
		// 2026-05-11 is a Monday -> ISO week 2026-W20.
		{Date: "2026-05-11", Project: "acme/a", Repo: "/dev/a", Model: "opus", InputTokens: 10, OutputTokens: 20, Cost: 1.0},
		{Date: "2026-05-11", Project: "acme/b", Repo: "/dev/b", Model: "opus", InputTokens: 5, OutputTokens: 5, Cost: 0.5},
		// 2026-05-12 same week.
		{Date: "2026-05-12", Project: "acme/a", Repo: "/dev/a", Model: "sonnet", InputTokens: 1, OutputTokens: 2, Cost: 0.25},
		// 2026-05-18 is the next Monday -> ISO week 2026-W21.
		{Date: "2026-05-18", Project: "acme/a", Repo: "/dev/a", Model: "opus", InputTokens: 100, OutputTokens: 200, Cost: 4.0},
	}
}

func TestAggregateByDay(t *testing.T) {
	agg := Aggregate(sampleRows())

	if len(agg.Days) != 3 {
		t.Fatalf("days = %d, want 3", len(agg.Days))
	}
	// Days must be sorted ascending.
	if agg.Days[0].Date != "2026-05-11" || agg.Days[2].Date != "2026-05-18" {
		t.Errorf("days not sorted: %v", agg.Days)
	}
	// 2026-05-11 cost = 1.0 + 0.5.
	if agg.Days[0].Cost != 1.5 {
		t.Errorf("day 2026-05-11 cost = %v, want 1.5", agg.Days[0].Cost)
	}
}

func TestAggregateByWeek(t *testing.T) {
	agg := Aggregate(sampleRows())

	if len(agg.Weeks) != 2 {
		t.Fatalf("weeks = %d, want 2", len(agg.Weeks))
	}
	if agg.Weeks[0].Week != "2026-W20" {
		t.Errorf("first week = %q, want 2026-W20", agg.Weeks[0].Week)
	}
	// W20 cost = 1.0 + 0.5 + 0.25.
	if agg.Weeks[0].Cost != 1.75 {
		t.Errorf("week 2026-W20 cost = %v, want 1.75", agg.Weeks[0].Cost)
	}
	if agg.Weeks[1].Week != "2026-W21" || agg.Weeks[1].Cost != 4.0 {
		t.Errorf("second week = %+v, want 2026-W21 / 4.0", agg.Weeks[1])
	}
}

func TestAggregateByProject(t *testing.T) {
	agg := Aggregate(sampleRows())

	if len(agg.Projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(agg.Projects))
	}
	// Projects sorted by cost descending: acme/a (1.0+0.25+4.0=5.25) then acme/b (0.5).
	if agg.Projects[0].Project != "acme/a" || agg.Projects[0].Cost != 5.25 {
		t.Errorf("top project = %+v, want acme/a / 5.25", agg.Projects[0])
	}
	if agg.Projects[1].Project != "acme/b" {
		t.Errorf("second project = %q, want acme/b", agg.Projects[1].Project)
	}
}

func TestAggregateProjectRepoAmbiguity(t *testing.T) {
	// Same project name, two distinct repo paths -> Repo must end up "".
	rows := []export.DailyRow{
		{Date: "2026-05-11", Project: "acme/a", Repo: "/dev/clone1", Model: "opus", Cost: 1.0},
		{Date: "2026-05-12", Project: "acme/a", Repo: "/dev/clone2", Model: "opus", Cost: 1.0},
	}
	agg := Aggregate(rows)
	if len(agg.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(agg.Projects))
	}
	if agg.Projects[0].Repo != "" {
		t.Errorf("Repo = %q, want empty (project spans two repo paths)", agg.Projects[0].Repo)
	}

	// A project with a single repo path keeps it.
	single := Aggregate([]export.DailyRow{
		{Date: "2026-05-11", Project: "acme/b", Repo: "/dev/only", Model: "opus", Cost: 1.0},
	})
	if single.Projects[0].Repo != "/dev/only" {
		t.Errorf("single-repo project Repo = %q, want /dev/only", single.Projects[0].Repo)
	}
}

func TestAggregatePerProjectSeriesForLines(t *testing.T) {
	agg := Aggregate(sampleRows())

	// ProjectKeys drives the per-project line series; sorted by total cost desc.
	if len(agg.ProjectKeys) != 2 || agg.ProjectKeys[0] != "acme/a" || agg.ProjectKeys[1] != "acme/b" {
		t.Fatalf("projectKeys = %v, want [acme/a acme/b]", agg.ProjectKeys)
	}
	// Each day carries a per-project cost breakdown keyed by project name.
	d0 := agg.Days[0] // 2026-05-11
	if d0.CostByProject["acme/a"] != 1.0 || d0.CostByProject["acme/b"] != 0.5 {
		t.Errorf("day 2026-05-11 CostByProject = %v", d0.CostByProject)
	}
}

func TestAggregatePerProjectTokenSeries(t *testing.T) {
	agg := Aggregate(sampleRows())

	// 2026-05-11: acme/a has 10+20 i/o tokens, acme/b has 5+5.
	d0 := agg.Days[0]
	if d0.TokensByProject["acme/a"] != 30 || d0.TokensByProject["acme/b"] != 10 {
		t.Errorf("day 2026-05-11 TokensByProject = %v", d0.TokensByProject)
	}
	// Week 2026-W20: acme/a = 30 (05-11) + 3 (05-12) = 33.
	w0 := agg.Weeks[0]
	if w0.TokensByProject["acme/a"] != 33 {
		t.Errorf("week 2026-W20 acme/a tokens = %d, want 33", w0.TokensByProject["acme/a"])
	}
}

func TestAggregateTotals(t *testing.T) {
	agg := Aggregate(sampleRows())
	// Total cost = 1.0 + 0.5 + 0.25 + 4.0.
	if agg.TotalCost != 5.75 {
		t.Errorf("TotalCost = %v, want 5.75", agg.TotalCost)
	}
	// Total tokens = input+output across all rows = (10+20)+(5+5)+(1+2)+(100+200).
	if agg.TotalTokens != 343 {
		t.Errorf("TotalTokens = %v, want 343", agg.TotalTokens)
	}
}

func TestAggregateEmpty(t *testing.T) {
	agg := Aggregate(nil)
	if len(agg.Days) != 0 || len(agg.Weeks) != 0 || len(agg.Projects) != 0 {
		t.Errorf("empty input should yield empty aggregate, got %+v", agg)
	}
	if agg.TotalCost != 0 {
		t.Errorf("empty TotalCost = %v, want 0", agg.TotalCost)
	}
}
