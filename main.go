package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/petertrost/claude-code-token-metrics/internal/analyze"
	"github.com/petertrost/claude-code-token-metrics/internal/ccusage"
	"github.com/petertrost/claude-code-token-metrics/internal/paths"
	"github.com/petertrost/claude-code-token-metrics/internal/repo"
	"github.com/petertrost/claude-code-token-metrics/internal/setup"
	"github.com/petertrost/claude-code-token-metrics/internal/snapshot"
)

const usage = `claude-code-token-metrics — durable Claude Code token-usage capture & analysis

Usage:
  claude-code-token-metrics setup     Install the SessionStart/SessionEnd hooks and launchd sweep job
  claude-code-token-metrics sweep     Snapshot ~/.claude/projects into the local store
  claude-code-token-metrics analyze   Resolve repos, run ccusage, write CSV/JSON
`

func runSweep() error {
	if err := snapshot.Sweep(paths.ClaudeProjectsDir(), paths.StoreProjectsDir()); err != nil {
		return err
	}
	// Capture any worktree -> repo pairs that are live right now into the
	// durable index, so they survive the instance being killed later.
	added, err := repo.MergeSquadStateIntoRepoMap(paths.SquadStateFile(), paths.RepoMapFile())
	if err != nil {
		return err
	}
	if added > 0 {
		fmt.Printf("sweep: recorded %d new worktree->repo mapping(s)\n", added)
	}
	return nil
}

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	outDir := fs.String("out", "out", "directory for the CSV/JSON output files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := analyze.Run(analyze.Config{
		StoreRoot:      paths.StoreRoot(),
		SquadStatePath: paths.SquadStateFile(),
		RepoMapPath:    paths.RepoMapFile(),
		ConfigPath:     paths.ConfigFile(),
		OutDir:         *outDir,
		CCUsage:        ccusage.DefaultRunner(),
	})
	if err != nil {
		return err
	}
	fmt.Printf("analyze: wrote %d session rows and %d daily rows to %s/\n",
		len(res.Sessions), len(res.Daily), *outDir)
	return nil
}

func runSetup() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// The hook scripts ship in a hooks/ directory. Locating it relative to the
	// binary is unreliable when running from source, so honor the CCTM_HOOK_DIR
	// env var as an override; otherwise assume hooks/ sits beside the binary.
	hookDir := os.Getenv("CCTM_HOOK_DIR")
	if hookDir == "" {
		hookDir = filepath.Join(filepath.Dir(exe), "hooks")
	}
	hooksDest := filepath.Join(paths.ToolDir(), "hooks")
	summary, err := setup.Run(setup.Config{
		SettingsPath:           paths.ClaudeSettingsFile(),
		SessionEndScriptSrc:    filepath.Join(hookDir, "snapshot-transcript.sh"),
		SessionEndScriptDest:   filepath.Join(hooksDest, "snapshot-transcript.sh"),
		SessionStartScriptSrc:  filepath.Join(hookDir, "record-repo.sh"),
		SessionStartScriptDest: filepath.Join(hooksDest, "record-repo.sh"),
		HomeDir:                paths.HomeDir(),
		BinPath:                exe,
	})
	if summary != "" {
		fmt.Println(summary)
	}
	return err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "setup":
		if err := runSetup(); err != nil {
			fmt.Fprintln(os.Stderr, "setup:", err)
			os.Exit(1)
		}
	case "sweep":
		if err := runSweep(); err != nil {
			fmt.Fprintln(os.Stderr, "sweep:", err)
			os.Exit(1)
		}
		fmt.Println("sweep: snapshot store updated")
	case "analyze":
		if err := runAnalyze(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "analyze:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
