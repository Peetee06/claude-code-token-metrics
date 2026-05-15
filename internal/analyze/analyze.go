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
	RepoMapPath    string  // path to the durable cwd -> repo index
	ConfigPath     string  // path to the tool config.json
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
	// 1. Build a project-dir -> resolved-repo map. The resolver draws on the
	//    durable repo-map index, the volatile claude-squad state, and an
	//    optional Squad-default fallback from config.
	squadIdx, err := repo.LoadSquadState(cfg.SquadStatePath)
	if err != nil {
		return nil, err
	}
	repoMap, err := repo.LoadRepoMap(cfg.RepoMapPath)
	if err != nil {
		return nil, err
	}
	toolCfg, err := repo.LoadConfig(cfg.ConfigPath)
	if err != nil {
		return nil, err
	}
	resolver := repo.NewResolver(repo.ResolverSources{
		RepoMap:          repoMap,
		SquadState:       squadIdx,
		SquadDefaultRepo: toolCfg.SquadDefaultRepo,
	})
	repoByProject := resolveAllProjects(filepath.Join(cfg.StoreRoot, "projects"), resolver)

	// Derive a clean project name for each distinct resolved repo. Done once
	// per repo here so ProjectName's git-remote lookup is not repeated per row.
	projectByRepo := map[string]string{}
	for _, r := range repoByProject {
		if _, done := projectByRepo[r]; !done {
			projectByRepo[r] = repo.ProjectName(r, toolCfg.ProjectAliases)
		}
	}

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
		Daily:    buildDailyRows(daily, repoByProject, projectByRepo),
		Sessions: buildSessionRows(session, repoByProject, projectByRepo),
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

// projectFor returns the clean project name for a repo path, defaulting to
// UnknownRepo for repos with no derived project name.
func projectFor(projectByRepo map[string]string, repoPath string) string {
	if p, ok := projectByRepo[repoPath]; ok && p != "" {
		return p
	}
	return repo.UnknownRepo
}

// buildDailyRows flattens ccusage daily output to (date, project, model) rows.
// ccusage separates by project-dir and date; we map project-dir -> repo ->
// project name and fan out the model breakdown. Project is the primary
// rollup key, so every repo path of one project (dev clone, CI runner dirs)
// is summed into a single row per (date, project, model). Repo is set only
// when the project maps to exactly one repo path, else left blank.
func buildDailyRows(rep *ccusage.DailyReport, repoByProject, projectByRepo map[string]string) []export.DailyRow {
	type key struct{ date, project, model string }
	acc := map[key]*export.DailyRow{}
	reposSeen := map[string]map[string]bool{} // project -> set of repo paths
	for projectKey, days := range rep.Projects {
		r := repoFor(repoByProject, projectKey)
		proj := projectFor(projectByRepo, r)
		if reposSeen[proj] == nil {
			reposSeen[proj] = map[string]bool{}
		}
		reposSeen[proj][r] = true
		for _, d := range days {
			for _, mb := range d.ModelBreakdowns {
				k := key{d.Date, proj, mb.ModelName}
				row := acc[k]
				if row == nil {
					row = &export.DailyRow{Date: d.Date, Project: proj, Model: mb.ModelName}
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
		row.Repo = singleRepo(reposSeen[row.Project])
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// singleRepo returns the sole repo path in the set, or "" if the project
// spans zero or several repo paths (in which case the path is ambiguous and
// project is the only meaningful identifier).
func singleRepo(repos map[string]bool) string {
	if len(repos) != 1 {
		return ""
	}
	for r := range repos {
		return r
	}
	return ""
}

// buildSessionRows flattens ccusage session output to (project, repo,
// project-dir, model) rows. ccusage keys sessions by project directory, so
// ProjectDir is the ccusage sessionId; Repo and Project are derived from it.
func buildSessionRows(rep *ccusage.SessionReport, repoByProject, projectByRepo map[string]string) []export.SessionRow {
	out := []export.SessionRow{}
	for _, s := range rep.Sessions {
		r := repoFor(repoByProject, s.SessionID)
		proj := projectFor(projectByRepo, r)
		for _, mb := range s.ModelBreakdowns {
			out = append(out, export.SessionRow{
				Project:             proj,
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
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		if out[i].ProjectDir != out[j].ProjectDir {
			return out[i].ProjectDir < out[j].ProjectDir
		}
		return out[i].Model < out[j].Model
	})
	return out
}
