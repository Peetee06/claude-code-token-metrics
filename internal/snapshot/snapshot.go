// Package snapshot copies live Claude Code transcripts into the durable store.
package snapshot

import (
	"fmt"
	"os"
	"os/exec"
)

// Sweep mirrors srcProjects (typically ~/.claude/projects) into dstProjects
// (the store's projects/ directory) using rsync. It is additive and
// idempotent: rsync copies only changed files and the tool never deletes
// snapshots, so --delete is intentionally NOT passed.
func Sweep(srcProjects, dstProjects string) error {
	if _, err := os.Stat(srcProjects); err != nil {
		return fmt.Errorf("source projects dir %q not found: %w", srcProjects, err)
	}
	if err := os.MkdirAll(dstProjects, 0o755); err != nil {
		return fmt.Errorf("creating store dir %q: %w", dstProjects, err)
	}
	// Trailing slash on src copies the *contents* of srcProjects into
	// dstProjects, preserving the per-project subdirectory structure.
	cmd := exec.Command("rsync", "-a",
		"--include=*/", "--include=*.jsonl", "--exclude=*",
		srcProjects+"/", dstProjects+"/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w\n%s", err, out)
	}
	return nil
}
