// Package export writes analysis results as CSV and JSON files.
package export

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// SessionRow is one row of sessions.{csv,json}: one (project, repo, model)
// triple. Project is the primary identifier (a clean owner/repo name); Repo
// is the underlying repo path, kept as secondary detail. ccusage cannot break
// usage down per transcript UUID, so ProjectDir is the finest ccusage key.
type SessionRow struct {
	Project             string  `json:"project"`
	Repo                string  `json:"repo"`
	ProjectDir          string  `json:"project_dir"`
	Model               string  `json:"model"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	Cost                float64 `json:"cost"`
}

// DailyRow is one row of daily.{csv,json}: one (date, project, repo, model)
// tuple. Project is the primary identifier; Repo is secondary detail.
type DailyRow struct {
	Date                string  `json:"date"`
	Project             string  `json:"project"`
	Repo                string  `json:"repo"`
	Model               string  `json:"model"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	Cost                float64 `json:"cost"`
}

// WriteSessions writes sessions.csv and sessions.json into dir.
func WriteSessions(dir string, rows []SessionRow) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	header := []string{"project", "repo", "project_dir", "model", "input_tokens",
		"output_tokens", "cache_creation_tokens", "cache_read_tokens", "cost"}
	recs := make([][]string, len(rows))
	for i, r := range rows {
		recs[i] = []string{r.Project, r.Repo, r.ProjectDir, r.Model,
			itoa(r.InputTokens), itoa(r.OutputTokens),
			itoa(r.CacheCreationTokens), itoa(r.CacheReadTokens),
			ftoa(r.Cost)}
	}
	if err := writeCSV(filepath.Join(dir, "sessions.csv"), header, recs); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "sessions.json"), rows)
}

// WriteDaily writes daily.csv and daily.json into dir.
func WriteDaily(dir string, rows []DailyRow) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	header := []string{"date", "project", "repo", "model", "input_tokens",
		"output_tokens", "cache_creation_tokens", "cache_read_tokens", "cost"}
	recs := make([][]string, len(rows))
	for i, r := range rows {
		recs[i] = []string{r.Date, r.Project, r.Repo, r.Model,
			itoa(r.InputTokens), itoa(r.OutputTokens),
			itoa(r.CacheCreationTokens), itoa(r.CacheReadTokens),
			ftoa(r.Cost)}
	}
	if err := writeCSV(filepath.Join(dir, "daily.csv"), header, recs); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "daily.json"), rows)
}

func writeCSV(path string, header []string, records [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := w.WriteAll(records); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func itoa(n int64) string   { return strconv.FormatInt(n, 10) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
