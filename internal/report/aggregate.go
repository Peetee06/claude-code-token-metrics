// Package report turns analyze's daily.json into a self-contained HTML report.
package report

import (
	"sort"
	"time"

	"github.com/petertrost/claude-code-token-metrics/internal/export"
)

// DayBucket is one calendar day's usage, with per-project cost and token
// breakdowns so the HTML can plot one line per project for either metric.
type DayBucket struct {
	Date            string             `json:"date"`
	Cost            float64            `json:"cost"`
	Tokens          int64              `json:"tokens"`
	CostByProject   map[string]float64 `json:"costByProject"`
	TokensByProject map[string]int64   `json:"tokensByProject"`
}

// WeekBucket is one ISO week's usage, same shape as DayBucket.
type WeekBucket struct {
	Week            string             `json:"week"` // ISO year-week, e.g. "2026-W20"
	Cost            float64            `json:"cost"`
	Tokens          int64              `json:"tokens"`
	CostByProject   map[string]float64 `json:"costByProject"`
	TokensByProject map[string]int64   `json:"tokensByProject"`
}

// ProjectBucket is one project's lifetime total. Repo is the underlying repo
// path when the project maps to exactly one, else "" (secondary detail only).
type ProjectBucket struct {
	Project string  `json:"project"`
	Repo    string  `json:"repo"`
	Cost    float64 `json:"cost"`
	Tokens  int64   `json:"tokens"`
}

// Aggregation is everything the HTML template needs.
type Aggregation struct {
	Days        []DayBucket     `json:"days"`
	Weeks       []WeekBucket    `json:"weeks"`
	Projects    []ProjectBucket `json:"projects"`
	ProjectKeys []string        `json:"projectKeys"` // line-series order, cost desc
	TotalCost   float64         `json:"totalCost"`
	TotalTokens int64           `json:"totalTokens"`
}

// isoWeek returns the ISO year-week label (e.g. "2026-W20") for a YYYY-MM-DD
// date string. An unparseable date is bucketed under "unknown".
func isoWeek(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "unknown"
	}
	year, week := t.ISOWeek()
	// %02d on the week keeps labels sortable as strings.
	return formatWeek(year, week)
}

func formatWeek(year, week int) string {
	w := []byte{byte('0' + week/10), byte('0' + week%10)}
	return itoa(year) + "-W" + string(w)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Aggregate rolls daily rows up into day, week, and project buckets, keyed by
// project name. It is the pure, testable core of the report; the HTML template
// only formats the result.
func Aggregate(rows []export.DailyRow) Aggregation {
	var agg Aggregation

	dayIdx := map[string]*DayBucket{}
	weekIdx := map[string]*WeekBucket{}
	projIdx := map[string]*ProjectBucket{}

	for _, r := range rows {
		tokens := r.InputTokens + r.OutputTokens
		agg.TotalCost += r.Cost
		agg.TotalTokens += tokens

		// Day bucket.
		d := dayIdx[r.Date]
		if d == nil {
			d = &DayBucket{
				Date:            r.Date,
				CostByProject:   map[string]float64{},
				TokensByProject: map[string]int64{},
			}
			dayIdx[r.Date] = d
		}
		d.Cost += r.Cost
		d.Tokens += tokens
		d.CostByProject[r.Project] += r.Cost
		d.TokensByProject[r.Project] += tokens

		// Week bucket.
		wk := isoWeek(r.Date)
		w := weekIdx[wk]
		if w == nil {
			w = &WeekBucket{
				Week:            wk,
				CostByProject:   map[string]float64{},
				TokensByProject: map[string]int64{},
			}
			weekIdx[wk] = w
		}
		w.Cost += r.Cost
		w.Tokens += tokens
		w.CostByProject[r.Project] += r.Cost
		w.TokensByProject[r.Project] += tokens

		// Project bucket.
		p := projIdx[r.Project]
		if p == nil {
			p = &ProjectBucket{Project: r.Project, Repo: r.Repo}
			projIdx[r.Project] = p
		} else if p.Repo != r.Repo {
			// Project spans more than one repo path -> repo is ambiguous.
			p.Repo = ""
		}
		p.Cost += r.Cost
		p.Tokens += tokens
	}

	for _, d := range dayIdx {
		agg.Days = append(agg.Days, *d)
	}
	sort.Slice(agg.Days, func(i, j int) bool { return agg.Days[i].Date < agg.Days[j].Date })

	for _, w := range weekIdx {
		agg.Weeks = append(agg.Weeks, *w)
	}
	sort.Slice(agg.Weeks, func(i, j int) bool { return agg.Weeks[i].Week < agg.Weeks[j].Week })

	for _, p := range projIdx {
		agg.Projects = append(agg.Projects, *p)
	}
	sort.Slice(agg.Projects, func(i, j int) bool {
		if agg.Projects[i].Cost != agg.Projects[j].Cost {
			return agg.Projects[i].Cost > agg.Projects[j].Cost
		}
		return agg.Projects[i].Project < agg.Projects[j].Project
	})

	// Project keys for the line series follow project order (cost desc).
	for _, p := range agg.Projects {
		agg.ProjectKeys = append(agg.ProjectKeys, p.Project)
	}

	return agg
}
