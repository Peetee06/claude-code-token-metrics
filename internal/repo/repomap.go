package repo

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// LoadRepoMap reads the durable cwd -> origin-repo index written by the
// SessionStart hook (and refreshed by sweep). A missing file yields an empty
// index and no error, so callers work on a machine that has never run the
// hook. The index survives Claude Squad worktree deletion, which is the whole
// point: it is consulted before the volatile claude-squad state.json.
func LoadRepoMap(path string) (map[string]string, error) {
	idx := map[string]string{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return idx, nil
	}
	if err != nil {
		return idx, err
	}
	if len(data) == 0 {
		return idx, nil
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, err
	}
	return idx, nil
}

// WriteRepoMap writes the cwd -> origin-repo index as indented JSON, creating
// the parent directory if needed.
func WriteRepoMap(path string, idx map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}
