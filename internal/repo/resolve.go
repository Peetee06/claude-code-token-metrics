package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UnknownRepo is the bucket for transcripts whose repo cannot be resolved.
const UnknownRepo = "unknown"

// squadWorktreeMarker identifies a cwd that lives inside a Claude Squad
// worktree, used by the squad-default fallback.
const squadWorktreeMarker = "/.claude-squad/worktrees/"

// ResolverSources holds every input the Resolver draws on. All fields are
// optional: a zero-value ResolverSources still resolves live git repos and
// otherwise returns UnknownRepo.
type ResolverSources struct {
	// RepoMap is the durable cwd -> origin-repo index written by the
	// SessionStart hook (see LoadRepoMap). It survives worktree deletion.
	RepoMap map[string]string
	// SquadState is the worktree_path -> repo_path index from the live
	// claude-squad state.json (see LoadSquadState). Volatile: an entry
	// disappears when its instance is killed.
	SquadState map[string]string
	// SquadDefaultRepo, when non-empty, is the repo every Claude Squad
	// worktree falls back to when nothing more precise resolves it.
	SquadDefaultRepo string
}

// Resolver resolves transcripts to canonical origin-repo paths.
type Resolver struct {
	src ResolverSources
}

// NewResolver builds a Resolver from the given sources. Pass a zero-value
// ResolverSources if no index or config is available.
func NewResolver(src ResolverSources) *Resolver {
	return &Resolver{src: src}
}

// Resolve returns the canonical origin-repo path for one transcript.
// projectDir is the snapshot project-dir name (currently unused but kept for
// future heuristics and a stable signature). transcriptPath is the JSONL file.
// Resolution order, most precise first:
//  1. Read the first cwd from the transcript.
//  2. If cwd is a live path inside a git repo -> git common dir's parent
//     (folds a worktree back to its origin repo).
//  3. If cwd is in the durable repo-map index -> that repo. Survives the
//     worktree being deleted, so this is the primary fix for dead worktrees.
//  4. If cwd is in the live claude-squad state index -> that repo. Kept as a
//     fallback for instances alive now but never seen by the SessionStart hook.
//  5. If cwd is under a Claude Squad worktrees dir and SquadDefaultRepo is
//     configured -> that repo. Recovers historical sessions whose worktrees
//     were deleted before the hook existed.
//  6. Otherwise -> UnknownRepo.
func (r *Resolver) Resolve(projectDir, transcriptPath string) string {
	cwd, err := FirstCwd(transcriptPath)
	if err != nil || cwd == "" {
		return UnknownRepo
	}

	// Step 2: live path -> git.
	if _, statErr := os.Stat(cwd); statErr == nil {
		if repo := gitOriginRepo(cwd); repo != "" {
			return repo
		}
	}

	// Step 3: durable repo-map index.
	if repo, ok := r.src.RepoMap[cwd]; ok {
		return repo
	}

	// Step 4: live claude-squad state index.
	if repo, ok := r.src.SquadState[cwd]; ok {
		return repo
	}

	// Step 5: blanket Squad-worktree fallback.
	if r.src.SquadDefaultRepo != "" && strings.Contains(cwd, squadWorktreeMarker) {
		return r.src.SquadDefaultRepo
	}

	return UnknownRepo
}

// gitOriginRepo returns the origin-repo top-level for a live directory,
// folding a worktree back to its main repo. Returns "" on any git failure.
func gitOriginRepo(dir string) string {
	// --git-common-dir points at the SHARED .git dir even from a worktree.
	common := gitOutput(dir, "rev-parse", "--git-common-dir")
	if common == "" {
		return ""
	}
	// common is typically ".git" or an absolute path ending in .git.
	// Resolve it to an absolute path, then take its parent = repo root.
	if !strings.HasPrefix(common, "/") {
		// relative to dir
		toplevel := gitOutput(dir, "rev-parse", "--show-toplevel")
		return toplevel
	}
	// absolute git dir -> the repo root is its parent directory
	return filepath.Dir(common)
}

// gitOutput runs `git -C dir <args...>` and returns trimmed stdout, or "".
func gitOutput(dir string, args ...string) string {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
