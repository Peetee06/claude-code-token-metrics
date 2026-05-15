// Package setup installs the SessionEnd hook and the launchd sweep job.
package setup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// MergeSessionEndHook adds a SessionEnd hook running scriptPath into the
// Claude Code settings file at settingsPath. It preserves every other key and
// every other hook event, and is idempotent: a SessionEnd entry already
// pointing at scriptPath is not duplicated. A missing settings file is
// created with just the hooks object.
func MergeSessionEndHook(settingsPath, scriptPath string) error {
	settings := map[string]any{}

	data, err := os.ReadFile(settingsPath)
	switch {
	case err == nil:
		if len(data) > 0 {
			if err := json.Unmarshal(data, &settings); err != nil {
				return err
			}
		}
	case errors.Is(err, fs.ErrNotExist):
		// settings will be created fresh below.
	default:
		return err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	// SessionEnd is an array of matcher-groups; each group has a hooks array.
	sessionEnd, _ := hooks["SessionEnd"].([]any)

	// Idempotency: bail if any entry already runs our script.
	if sessionEndContainsScript(sessionEnd, scriptPath) {
		return nil
	}

	newGroup := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": scriptPath},
		},
	}
	hooks["SessionEnd"] = append(sessionEnd, newGroup)
	settings["hooks"] = hooks

	return writeSettings(settingsPath, settings)
}

// sessionEndContainsScript reports whether any SessionEnd group already runs
// command == scriptPath.
func sessionEndContainsScript(sessionEnd []any, scriptPath string) bool {
	for _, g := range sessionEnd {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); cmd == scriptPath {
				return true
			}
		}
	}
	return false
}

// writeSettings writes settings as indented JSON, creating parent dirs.
func writeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}
