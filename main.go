package main

import (
	"fmt"
	"os"
)

const usage = `claude-code-token-metrics — durable Claude Code token-usage capture & analysis

Usage:
  claude-code-token-metrics setup     Install the SessionEnd hook and launchd sweep job
  claude-code-token-metrics sweep     Snapshot ~/.claude/projects into the local store
  claude-code-token-metrics analyze   Resolve repos, run ccusage, write CSV/JSON
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "setup":
		fmt.Println("setup: not yet implemented")
	case "sweep":
		fmt.Println("sweep: not yet implemented")
	case "analyze":
		fmt.Println("analyze: not yet implemented")
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
