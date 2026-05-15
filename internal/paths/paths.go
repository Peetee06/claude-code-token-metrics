// Package paths centralizes the well-known filesystem locations the tool uses.
// HomeDir is a package variable so tests can point it at a temp directory.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// HomeDir returns the user's home directory. Override in tests.
var HomeDir = func() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("paths: cannot determine home directory: %v", err))
	}
	return h
}

// ClaudeProjectsDir is the live Claude Code transcript directory.
func ClaudeProjectsDir() string {
	return filepath.Join(HomeDir(), ".claude", "projects")
}

// ClaudeSettingsFile is the Claude Code settings file edited by `setup`.
func ClaudeSettingsFile() string {
	return filepath.Join(HomeDir(), ".claude", "settings.json")
}

// StoreRoot is the snapshot store root. ccusage is invoked with
// CLAUDE_CONFIG_DIR set to this path, so it must contain a projects/ subdir.
func StoreRoot() string {
	return filepath.Join(HomeDir(), ".claude-token-metrics", "snapshots")
}

// StoreProjectsDir is the projects/ subdirectory ccusage expects.
func StoreProjectsDir() string {
	return filepath.Join(StoreRoot(), "projects")
}

// SquadStateFile is the Claude Squad state file used for worktree resolution.
func SquadStateFile() string {
	return filepath.Join(HomeDir(), ".claude-squad", "state.json")
}
