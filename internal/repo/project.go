package repo

import (
	"path"
	"strings"
)

// ProjectName derives a clean, stable project identifier for a resolved repo
// path. It is the primary grouping key for reporting; the repo path itself is
// kept only as secondary detail.
//
// Resolution order:
//  1. The UnknownRepo sentinel passes through unchanged.
//  2. An explicit entry in aliases (keyed by repo path) wins — the user's
//     mapping is authoritative, e.g. for paths whose worktree is now deleted.
//  3. The repo's git remote, parsed to "owner/repo". This collapses the same
//     project checked out at several paths (dev clone, CI runner dirs).
//  4. Fallback: the basename of the repo path.
func ProjectName(repoPath string, aliases map[string]string) string {
	if repoPath == "" || repoPath == UnknownRepo {
		return repoPath
	}
	if alias, ok := aliases[repoPath]; ok && alias != "" {
		return alias
	}
	if url := gitOutput(repoPath, "config", "--get", "remote.origin.url"); url != "" {
		if proj := parseRemoteToProject(url); proj != "" {
			return proj
		}
	}
	return path.Base(repoPath)
}

// parseRemoteToProject extracts "owner/repo" from a git remote URL, tolerating
// https, ssh scp-style (git@host:owner/repo.git), and ssh:// URL forms, with
// or without a trailing ".git" or "/". Returns "" if no owner/repo pair can be
// extracted.
func parseRemoteToProject(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")

	// Normalize the scp-style "git@host:owner/repo" form to a slash path.
	if i := strings.Index(url, "@"); i >= 0 {
		if c := strings.Index(url[i:], ":"); c >= 0 {
			url = url[i+c+1:]
		}
	}
	// Strip any scheme://host/ prefix, leaving the path.
	if i := strings.Index(url, "://"); i >= 0 {
		rest := url[i+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			url = rest[slash+1:]
		}
	}

	// The last two path segments are owner/repo.
	segs := strings.Split(strings.Trim(url, "/"), "/")
	if len(segs) < 2 {
		return ""
	}
	owner := segs[len(segs)-2]
	repo := segs[len(segs)-1]
	if owner == "" || repo == "" {
		return ""
	}
	return owner + "/" + repo
}
