package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SweepLabel is the launchd job label; also the plist filename stem.
const SweepLabel = "com.petertrost.claude-code-token-metrics.sweep"

// sweepIntervalSeconds is the StartInterval for the sweep job (4 hours).
const sweepIntervalSeconds = 14400

// RenderSweepPlist returns a launchd plist that runs `<binPath> sweep` every
// sweepIntervalSeconds.
func RenderSweepPlist(binPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>sweep</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, SweepLabel, binPath, sweepIntervalSeconds)
}

// InstallSweepJob writes the plist into ~/Library/LaunchAgents and (re)loads
// it with launchctl. homeDir is passed in so tests can use a temp dir.
func InstallSweepJob(homeDir, binPath string) (string, error) {
	agentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return "", err
	}
	plistPath := filepath.Join(agentsDir, SweepLabel+".plist")
	if err := os.WriteFile(plistPath, []byte(RenderSweepPlist(binPath)), 0o644); err != nil {
		return "", err
	}
	// Reload: unload first (ignore error if not loaded), then load.
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		// Non-fatal: the plist is on disk and will load at next login.
		return plistPath, fmt.Errorf("plist written to %s but launchctl load failed: %w", plistPath, err)
	}
	return plistPath, nil
}
