package main

import (
	"fmt"
	"os"

	"github.com/petertrost/claude-code-token-metrics/internal/paths"
	"github.com/petertrost/claude-code-token-metrics/internal/snapshot"
)

const usage = `claude-code-token-metrics — durable Claude Code token-usage capture & analysis

Usage:
  claude-code-token-metrics setup     Install the SessionEnd hook and launchd sweep job
  claude-code-token-metrics sweep     Snapshot ~/.claude/projects into the local store
  claude-code-token-metrics analyze   Resolve repos, run ccusage, write CSV/JSON
`

func runSweep() error {
	return snapshot.Sweep(paths.ClaudeProjectsDir(), paths.StoreProjectsDir())
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "setup":
		fmt.Println("setup: not yet implemented")
	case "sweep":
		if err := runSweep(); err != nil {
			fmt.Fprintln(os.Stderr, "sweep:", err)
			os.Exit(1)
		}
		fmt.Println("sweep: snapshot store updated")
	case "analyze":
		fmt.Println("analyze: not yet implemented")
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
