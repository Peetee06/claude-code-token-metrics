package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDaily(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily.json")
	content := `[
  {"date":"2026-05-11","project":"acme/a","repo":"/dev/a","model":"opus","input_tokens":10,"output_tokens":20,"cache_creation_tokens":0,"cache_read_tokens":0,"cost":1.0}
]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := LoadDaily(path)
	if err != nil {
		t.Fatalf("LoadDaily: %v", err)
	}
	if len(rows) != 1 || rows[0].Project != "acme/a" || rows[0].Cost != 1.0 {
		t.Errorf("LoadDaily = %+v", rows)
	}
}

func TestLoadDailyMissingFile(t *testing.T) {
	_, err := LoadDaily(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Error("LoadDaily on a missing file should return an error")
	}
}

func TestRenderProducesSelfContainedHTML(t *testing.T) {
	agg := Aggregate(sampleRows())
	html, err := Render(agg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	s := string(html)

	for _, want := range []string{
		"<!doctype html>",
		"<canvas",      // chart canvases
		"chart.js",     // the charting library is referenced
		"2026-05-11",   // a day label is embedded
		"2026-W20",     // a week label is embedded
		"acme/a",       // a project label is embedded
		"metricToggle", // cost/tokens control
		"scaleToggle",  // linear/log control
		"presetToggle", // 7d/30d/all control
		"fromSel",      // custom range dropdowns
	} {
		if !strings.Contains(strings.ToLower(s), strings.ToLower(want)) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
	// The aggregation must be embedded as inline JSON so the file is
	// self-contained (no fetch of a separate data file).
	if !strings.Contains(s, `"days"`) || !strings.Contains(s, `"weeks"`) {
		t.Error("rendered HTML does not embed the aggregation JSON")
	}
	// Both metric breakdowns must be embedded so the cost/tokens toggle works.
	if !strings.Contains(s, `"costByProject"`) || !strings.Contains(s, `"tokensByProject"`) {
		t.Error("rendered HTML does not embed both cost and token breakdowns")
	}
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()
	// Seed a daily.json that WriteReport will read.
	daily := filepath.Join(dir, "daily.json")
	content := `[{"date":"2026-05-11","project":"acme/a","repo":"/dev/a","model":"opus","input_tokens":10,"output_tokens":20,"cache_creation_tokens":0,"cache_read_tokens":0,"cost":1.0}]`
	if err := os.WriteFile(daily, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath, err := WriteReport(dir)
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	if outPath != filepath.Join(dir, "report.html") {
		t.Errorf("report path = %q", outPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("report.html not written: %v", err)
	}
	if !strings.Contains(string(data), "acme/a") {
		t.Error("report.html missing expected data")
	}
}

func TestWriteReportMissingDaily(t *testing.T) {
	_, err := WriteReport(t.TempDir()) // no daily.json present
	if err == nil {
		t.Error("WriteReport should error when daily.json is absent")
	}
}
