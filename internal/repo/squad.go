package repo

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// squadState mirrors the subset of ~/.claude-squad/state.json we use.
type squadState struct {
	Instances []struct {
		Worktree struct {
			RepoPath     string `json:"repo_path"`
			WorktreePath string `json:"worktree_path"`
		} `json:"worktree"`
	} `json:"instances"`
}

// LoadSquadState reads the Claude Squad state file and returns a lookup of
// worktree_path -> repo_path. A missing file yields an empty index and no
// error, so callers can run on machines without Claude Squad.
func LoadSquadState(path string) (map[string]string, error) {
	idx := map[string]string{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	var s squadState
	if err := json.Unmarshal(data, &s); err != nil {
		return idx, err
	}
	for _, inst := range s.Instances {
		wt := inst.Worktree
		if wt.WorktreePath != "" && wt.RepoPath != "" {
			idx[wt.WorktreePath] = wt.RepoPath
		}
	}
	return idx, nil
}
