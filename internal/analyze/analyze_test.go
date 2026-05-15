package analyze

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
)

// fakeCCUsage returns canned reports, ignoring the store path.
type fakeCCUsage struct {
	daily   *ccusage.DailyReport
	session *ccusage.SessionReport
}

func (f fakeCCUsage) Daily(string) (*ccusage.DailyReport, error)     { return f.daily, nil }
func (f fakeCCUsage) Session(string) (*ccusage.SessionReport, error) { return f.session, nil }

// runGit runs `git -C dir args...` and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeLine writes a single-line file.
func writeLine(t *testing.T, path, line string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunJoinsCCUsageToRepos(t *testing.T) {
	store := t.TempDir()
	projects := filepath.Join(store, "projects")

	// One project dir whose transcript cwd is a live git repo.
	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "t@t.t")
	runGit(t, repoDir, "config", "user.name", "t")

	projDir := filepath.Join(projects, "-proj-a")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLine(t, filepath.Join(projDir, "s.jsonl"),
		`{"type":"assistant","cwd":"`+repoDir+`"}`)

	fake := fakeCCUsage{
		daily: &ccusage.DailyReport{Projects: map[string][]ccusage.DailyRow{
			"-proj-a": {{
				Date: "2026-05-15",
				ModelBreakdowns: []ccusage.ModelBreakdown{
					{ModelName: "claude-opus-4-7", InputTokens: 10,
						OutputTokens: 20, CacheCreationTokens: 1,
						CacheReadTokens: 2, Cost: 0.5},
				},
			}},
		}},
		session: &ccusage.SessionReport{Sessions: []ccusage.SessionEntry{{
			SessionID: "-proj-a",
			ModelBreakdowns: []ccusage.ModelBreakdown{
				{ModelName: "claude-opus-4-7", InputTokens: 10,
					OutputTokens: 20, CacheCreationTokens: 1,
					CacheReadTokens: 2, Cost: 0.5},
			},
		}}},
	}

	out := t.TempDir()
	res, err := Run(Config{
		StoreRoot:      store,
		SquadStatePath: filepath.Join(t.TempDir(), "no-squad.json"),
		OutDir:         out,
		CCUsage:        fake,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(res.Daily) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(res.Daily))
	}
	wantRepo, _ := filepath.EvalSymlinks(repoDir)
	gotRepo, _ := filepath.EvalSymlinks(res.Daily[0].Repo)
	if gotRepo != wantRepo {
		t.Errorf("daily row repo = %q, want %q", res.Daily[0].Repo, wantRepo)
	}
	if res.Daily[0].Model != "claude-opus-4-7" || res.Daily[0].InputTokens != 10 {
		t.Errorf("daily row = %+v", res.Daily[0])
	}
	if len(res.Sessions) != 1 {
		t.Errorf("session rows = %d, want 1", len(res.Sessions))
	}
	for _, name := range []string{"daily.csv", "daily.json", "sessions.csv", "sessions.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing output %s: %v", name, err)
		}
	}
}

func TestRunUnknownRepoBucket(t *testing.T) {
	store := t.TempDir()
	projects := filepath.Join(store, "projects")
	projDir := filepath.Join(projects, "-proj-x")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Transcript with no cwd -> unresolvable.
	writeLine(t, filepath.Join(projDir, "s.jsonl"), `{"type":"system"}`)

	fake := fakeCCUsage{
		daily: &ccusage.DailyReport{Projects: map[string][]ccusage.DailyRow{
			"-proj-x": {{Date: "2026-05-15", ModelBreakdowns: []ccusage.ModelBreakdown{
				{ModelName: "claude-haiku-4-5-20251001", InputTokens: 1},
			}}},
		}},
		session: &ccusage.SessionReport{},
	}
	res, err := Run(Config{
		StoreRoot:      store,
		SquadStatePath: filepath.Join(t.TempDir(), "none.json"),
		OutDir:         t.TempDir(),
		CCUsage:        fake,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Daily) != 1 || res.Daily[0].Repo != "unknown" {
		t.Errorf("expected one row bucketed as unknown, got %+v", res.Daily)
	}
}

func TestRunSumsProjectsResolvingToSameRepo(t *testing.T) {
	store := t.TempDir()
	projects := filepath.Join(store, "projects")

	// One git repo that BOTH project dirs will resolve to.
	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "t@t.t")
	runGit(t, repoDir, "config", "user.name", "t")

	// Two project dirs, two transcripts, both cwd -> the same repo.
	for _, name := range []string{"-proj-a", "-proj-b"} {
		pd := filepath.Join(projects, name)
		if err := os.MkdirAll(pd, 0o755); err != nil {
			t.Fatal(err)
		}
		writeLine(t, filepath.Join(pd, "s.jsonl"),
			`{"type":"assistant","cwd":"`+repoDir+`"}`)
	}

	// ccusage reports both project dirs with the same date + model.
	day := func() []ccusage.DailyRow {
		return []ccusage.DailyRow{{
			Date: "2026-05-15",
			ModelBreakdowns: []ccusage.ModelBreakdown{
				{ModelName: "claude-opus-4-7", InputTokens: 10,
					OutputTokens: 20, CacheCreationTokens: 1,
					CacheReadTokens: 2, Cost: 0.5},
			},
		}}
	}
	fake := fakeCCUsage{
		daily: &ccusage.DailyReport{Projects: map[string][]ccusage.DailyRow{
			"-proj-a": day(),
			"-proj-b": day(),
		}},
		session: &ccusage.SessionReport{},
	}

	res, err := Run(Config{
		StoreRoot:      store,
		SquadStatePath: filepath.Join(t.TempDir(), "none.json"),
		OutDir:         t.TempDir(),
		CCUsage:        fake,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Both project dirs resolve to the same repo, same date+model ->
	// exactly ONE daily row with summed tokens.
	if len(res.Daily) != 1 {
		t.Fatalf("daily rows = %d, want 1 (the two projects must collapse)", len(res.Daily))
	}
	row := res.Daily[0]
	if row.InputTokens != 20 || row.OutputTokens != 40 ||
		row.CacheCreationTokens != 2 || row.CacheReadTokens != 4 || row.Cost != 1.0 {
		t.Errorf("summed row = %+v; want input=20 output=40 cacheCreate=2 cacheRead=4 cost=1.0", row)
	}
}
