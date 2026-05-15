package repo

import (
	"regexp"
	"strings"
)

// hashSuffix matches the trailing _<hex> Claude Squad appends to worktree dirs.
var hashSuffix = regexp.MustCompile(`_[0-9a-f]{8,}$`)

const squadMarker = "/.claude-squad/worktrees/"

// NormalizeWorktreeBranch extracts the branch name from a Claude Squad
// worktree path by taking everything after ".claude-squad/worktrees/" and
// stripping the trailing _<hash> suffix. Returns "" for any path that is not
// a Claude Squad worktree. The branch may contain slashes (e.g. "fix/x").
func NormalizeWorktreeBranch(path string) string {
	i := strings.Index(path, squadMarker)
	if i < 0 {
		return ""
	}
	rest := path[i+len(squadMarker):]
	rest = hashSuffix.ReplaceAllString(rest, "")
	return rest
}
