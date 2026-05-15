// Package analyze joins ccusage usage data to resolved git repositories.
package analyze

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
	"github.com/petertrost/claude-code-token-metrics/internal/export"
	"github.com/petertrost/claude-code-token-metrics/internal/repo"
)

// CCUsage is the subset of ccusage the analyzer needs. ccusage.Runner
// satisfies it; tests pass a fake.
type CCUsage interface {
	Daily(storeRoot string) (*ccusage.DailyReport, error)
	Session(storeRoot string) (*ccusage.SessionReport, error)
}

// Config holds the inputs for one analyze run.
type Config struct {
	StoreRoot      string  // snapshot store root (contains projects/)
	SquadStatePath string  // path to claude-squad state.json
	OutDir         string  // directory to write the four output files
	CCUsage        CCUsage // ccusage implementation
}

// Result is the in-memory outcome of a run, also written to OutDir.
type Result struct {
	Sessions []export.SessionRow
	Daily    []export.DailyRow
}

// Run resolves repos for every project dir in the store, calls ccusage,
// joins the two, writes the four output files, and returns the rows.
func Run(cfg Config) (*Result, error) {
	// 1. Build a project-dir -> resolved-repo map.
	squadIdx, err := repo.LoadSquadState(cfg.SquadStatePath)
	if err != nil {
		return nil, err
	}
	resolver := repo.NewResolver(squadIdx)
	repoByProject := resolveAllProjects(filepath.Join(cfg.StoreRoot, "projects"), resolver)

	// 2. Run ccusage.
	daily, err := cfg.CCUsage.Daily(cfg.StoreRoot)
	if err != nil {
		return nil, err
	}
	session, err := cfg.CCUsage.Session(cfg.StoreRoot)
	if err != nil {
		return nil, err
	}

	// 3. Join and roll up.
	res := &Result{
		Daily:    buildDailyRows(daily, repoByProject),
		Sessions: buildSessionRows(session, repoByProject),
	}

	// 4. Write outputs.
	if err := export.WriteSessions(cfg.OutDir, res.Sessions); err != nil {
		return nil, err
	}
	if err := export.WriteDaily(cfg.OutDir, res.Daily); err != nil {
		return nil, err
	}
	return res, nil
}

// resolveAllProjects resolves every project directory under projectsRoot.
// A project dir with no transcripts, or one that resolves to nothing, maps
// to repo.UnknownRepo.
func resolveAllProjects(projectsRoot string, resolver *repo.Resolver) map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return out // store may not exist yet; callers treat all as unknown
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projDir := e.Name()
		out[projDir] = repo.UnknownRepo
		// Resolve using the first transcript that yields a real repo.
		files, _ := os.ReadDir(filepath.Join(projectsRoot, projDir))
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".jsonl" {
				continue
			}
			tp := filepath.Join(projectsRoot, projDir, f.Name())
			if r := resolver.Resolve(projDir, tp); r != repo.UnknownRepo {
				out[projDir] = r
				break
			}
		}
	}
	return out
}

// repoFor returns the resolved repo for a ccusage project key, defaulting to
// UnknownRepo for project keys not seen in the store.
func repoFor(repoByProject map[string]string, projectKey string) string {
	if r, ok := repoByProject[projectKey]; ok {
		return r
	}
	return repo.UnknownRepo
}

// buildDailyRows flattens ccusage daily output to (date, repo, model) rows.
// ccusage already separates by project and date; we map project->repo and
// fan out the model breakdown. Two project dirs resolving to the same repo
// are summed into one row per (date, repo, model).
func buildDailyRows(rep *ccusage.DailyReport, repoByProject map[string]string) []export.DailyRow {
	type key struct{ date, repo, model string }
	acc := map[key]*export.DailyRow{}
	for projectKey, days := range rep.Projects {
		r := repoFor(repoByProject, projectKey)
		for _, d := range days {
			for _, mb := range d.ModelBreakdowns {
				k := key{d.Date, r, mb.ModelName}
				row := acc[k]
				if row == nil {
					row = &export.DailyRow{Date: d.Date, Repo: r, Model: mb.ModelName}
					acc[k] = row
				}
				row.InputTokens += mb.InputTokens
				row.OutputTokens += mb.OutputTokens
				row.CacheCreationTokens += mb.CacheCreationTokens
				row.CacheReadTokens += mb.CacheReadTokens
				row.Cost += mb.Cost
			}
		}
	}
	out := make([]export.DailyRow, 0, len(acc))
	for _, row := range acc {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// buildSessionRows flattens ccusage session output to (project-dir, repo,
// model) rows. ccusage keys sessions by project directory, so ProjectDir is
// the ccusage sessionId.
func buildSessionRows(rep *ccusage.SessionReport, repoByProject map[string]string) []export.SessionRow {
	out := []export.SessionRow{}
	for _, s := range rep.Sessions {
		r := repoFor(repoByProject, s.SessionID)
		for _, mb := range s.ModelBreakdowns {
			out = append(out, export.SessionRow{
				Repo:                r,
				ProjectDir:          s.SessionID,
				Model:               mb.ModelName,
				InputTokens:         mb.InputTokens,
				OutputTokens:        mb.OutputTokens,
				CacheCreationTokens: mb.CacheCreationTokens,
				CacheReadTokens:     mb.CacheReadTokens,
				Cost:                mb.Cost,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		if out[i].ProjectDir != out[j].ProjectDir {
			return out[i].ProjectDir < out[j].ProjectDir
		}
		return out[i].Model < out[j].Model
	})
	return out
}
