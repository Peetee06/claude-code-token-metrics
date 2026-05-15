package repo

import "testing"

func TestParseRemoteToProject(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"https with .git", "https://github.com/trost-systems/Iakuvo.git", "trost-systems/Iakuvo"},
		{"https without .git", "https://github.com/trost-systems/Iakuvo", "trost-systems/Iakuvo"},
		{"ssh scp-style", "git@github.com:Peetee06/date-my-bestie.git", "Peetee06/date-my-bestie"},
		{"ssh url form", "ssh://git@github.com/Peetee06/repo.git", "Peetee06/repo"},
		{"trailing slash", "https://github.com/owner/repo/", "owner/repo"},
		{"not a url", "", ""},
		{"single segment", "https://example.com/lonely", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseRemoteToProject(c.url); got != c.want {
				t.Errorf("parseRemoteToProject(%q) = %q, want %q", c.url, got, c.want)
			}
		})
	}
}

func TestProjectNameViaAlias(t *testing.T) {
	aliases := map[string]string{
		"/Users/me/.claude-squad/worktrees/old": "trost-systems/Iakuvo",
	}
	// A path that is not a live git repo, but has an alias entry.
	got := ProjectName("/Users/me/.claude-squad/worktrees/old", aliases)
	if got != "trost-systems/Iakuvo" {
		t.Errorf("ProjectName via alias = %q, want trost-systems/Iakuvo", got)
	}
}

func TestProjectNameFallsBackToBasename(t *testing.T) {
	// No live git repo, no alias -> basename of the path.
	got := ProjectName("/Users/me/dev/some-deleted-repo", nil)
	if got != "some-deleted-repo" {
		t.Errorf("ProjectName fallback = %q, want some-deleted-repo", got)
	}
}

func TestProjectNameUnknownStaysUnknown(t *testing.T) {
	// The UnknownRepo sentinel must pass through unchanged.
	if got := ProjectName(UnknownRepo, nil); got != UnknownRepo {
		t.Errorf("ProjectName(unknown) = %q, want %q", got, UnknownRepo)
	}
}

func TestProjectNameLiveRepoUsesRemote(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/trost-systems/Iakuvo.git")
	got := ProjectName(dir, nil)
	if got != "trost-systems/Iakuvo" {
		t.Errorf("ProjectName from live remote = %q, want trost-systems/Iakuvo", got)
	}
}

// Alias takes precedence over a live remote: the user's explicit mapping wins.
func TestProjectNameAliasBeatsRemote(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/trost-systems/Iakuvo.git")
	aliases := map[string]string{dir: "custom/name"}
	if got := ProjectName(dir, aliases); got != "custom/name" {
		t.Errorf("ProjectName = %q, want alias custom/name", got)
	}
}
