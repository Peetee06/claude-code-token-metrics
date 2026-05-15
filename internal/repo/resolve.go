package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UnknownRepo is the bucket for transcripts whose repo cannot be resolved.
const UnknownRepo = "unknown"

// Resolver resolves transcripts to canonical origin-repo paths.
type Resolver struct {
	// squadIdx maps worktree_path -> repo_path from claude-squad state.
	squadIdx map[string]string
}

// NewResolver builds a Resolver from a squad worktree->repo index
// (see LoadSquadState). Pass an empty map if Claude Squad is not used.
func NewResolver(squadIdx map[string]string) *Resolver {
	return &Resolver{squadIdx: squadIdx}
}

// Resolve returns the canonical origin-repo path for one transcript.
// projectDir is the snapshot project-dir name (currently unused but kept for
// future heuristics and a stable signature). transcriptPath is the JSONL file.
// Resolution order:
//  1. Read the first cwd from the transcript.
//  2. If cwd is a live path inside a git repo -> git common dir's parent
//     (folds a worktree back to its origin repo).
//  3. If cwd is dead (Squad-deleted worktree) -> exact match in the squad index.
//  4. Otherwise -> UnknownRepo.
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

	// Step 3: dead worktree -> squad index exact match.
	if repo, ok := r.squadIdx[cwd]; ok {
		return repo
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
