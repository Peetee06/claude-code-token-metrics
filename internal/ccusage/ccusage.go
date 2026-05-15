// Package ccusage invokes the ccusage CLI and parses its JSON reports.
package ccusage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// ModelBreakdown is one model's token/cost contribution within a row.
// Field names match ccusage's JSON exactly.
type ModelBreakdown struct {
	ModelName           string  `json:"modelName"`
	InputTokens         int64   `json:"inputTokens"`
	OutputTokens        int64   `json:"outputTokens"`
	CacheCreationTokens int64   `json:"cacheCreationTokens"`
	CacheReadTokens     int64   `json:"cacheReadTokens"`
	Cost                float64 `json:"cost"`
}

// DailyRow is one project's usage on one date.
type DailyRow struct {
	Date            string           `json:"date"`
	ModelBreakdowns []ModelBreakdown `json:"modelBreakdowns"`
}

// DailyReport is the parsed `ccusage daily --instances --breakdown --json`.
// Projects maps a ccusage project-dir key to its daily rows.
type DailyReport struct {
	Projects map[string][]DailyRow `json:"projects"`
}

// SessionEntry is one project's lifetime usage (ccusage keys sessions by
// project directory, not by transcript UUID).
type SessionEntry struct {
	SessionID       string           `json:"sessionId"`
	LastActivity    string           `json:"lastActivity"`
	ModelBreakdowns []ModelBreakdown `json:"modelBreakdowns"`
}

// SessionReport is the parsed `ccusage session --instances --breakdown --json`.
type SessionReport struct {
	Sessions []SessionEntry `json:"sessions"`
}

// ParseDaily parses a ccusage daily JSON payload.
func ParseDaily(data []byte) (*DailyReport, error) {
	var r DailyReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing ccusage daily JSON: %w", err)
	}
	return &r, nil
}

// ParseSession parses a ccusage session JSON payload.
func ParseSession(data []byte) (*SessionReport, error) {
	var r SessionReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing ccusage session JSON: %w", err)
	}
	return &r, nil
}

// ErrNotInstalled is returned when the ccusage binary cannot be launched.
var ErrNotInstalled = errors.New(
	"ccusage not found — install it with `npm install -g ccusage` " +
		"(or ensure `npx ccusage` works) and re-run")

// Runner invokes ccusage. The Command field is the executable plus any
// leading args; for `npx ccusage` use []string{"npx", "ccusage"}.
type Runner struct {
	Command []string // e.g. ["ccusage"] or ["npx", "ccusage"]
}

// DefaultRunner uses `npx ccusage` so the tool works without a global install.
func DefaultRunner() *Runner {
	return &Runner{Command: []string{"npx", "ccusage"}}
}

// run executes ccusage with CLAUDE_CONFIG_DIR=storeRoot and the given args,
// returning stdout. A launch failure is reported as ErrNotInstalled.
func (r *Runner) run(storeRoot string, args ...string) ([]byte, error) {
	full := append(append([]string{}, r.Command[1:]...), args...)
	cmd := exec.Command(r.Command[0], full...)
	cmd.Env = append(cmd.Environ(), "CLAUDE_CONFIG_DIR="+storeRoot)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("ccusage exited %d: %s",
				exitErr.ExitCode(), exitErr.Stderr)
		}
		return nil, ErrNotInstalled
	}
	return out, nil
}

// Daily runs `ccusage daily --instances --breakdown --json`.
func (r *Runner) Daily(storeRoot string) (*DailyReport, error) {
	out, err := r.run(storeRoot,
		"daily", "--instances", "--breakdown", "--json")
	if err != nil {
		return nil, err
	}
	return ParseDaily(out)
}

// Session runs `ccusage session --instances --breakdown --json`.
func (r *Runner) Session(storeRoot string) (*SessionReport, error) {
	out, err := r.run(storeRoot,
		"session", "--instances", "--breakdown", "--json")
	if err != nil {
		return nil, err
	}
	return ParseSession(out)
}
