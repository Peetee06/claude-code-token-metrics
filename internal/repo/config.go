package repo

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// Config is the optional ~/.claude-token-metrics/config.json.
type Config struct {
	// SquadDefaultRepo, when set, is the repo every Claude Squad worktree
	// (a cwd under ".claude-squad/worktrees/") is attributed to when no more
	// precise resolution succeeds. It exists to recover historical sessions
	// whose worktrees were deleted before the SessionStart hook existed.
	SquadDefaultRepo string `json:"squad_default_repo"`

	// ProjectAliases maps a resolved repo path to an explicit project name.
	// It overrides git-remote derivation, so a path whose worktree is now
	// deleted (no remote to read) can still get a clean project identity.
	ProjectAliases map[string]string `json:"project_aliases"`
}

// LoadConfig reads the tool config. A missing file yields a zero-value Config
// and no error, so the tool runs fine without any config at all.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
