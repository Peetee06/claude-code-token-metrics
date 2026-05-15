package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSweepPlist(t *testing.T) {
	plist := RenderSweepPlist("/usr/local/bin/claude-code-token-metrics")

	for _, want := range []string{
		"<?xml",
		"com.petertrost.claude-code-token-metrics.sweep", // Label
		"/usr/local/bin/claude-code-token-metrics",       // binary path
		"<string>sweep</string>",                         // the argument
		"<key>StartInterval</key>",
		"<integer>14400</integer>", // 4 hours
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q\n---\n%s", want, plist)
		}
	}
}

func TestInstallSweepJobWritesPlist(t *testing.T) {
	home := t.TempDir()
	binPath := "/usr/local/bin/claude-code-token-metrics"

	plistPath, err := InstallSweepJob(home, binPath)
	// launchctl load may fail in a temp/CI environment; that is non-fatal
	// and InstallSweepJob still returns the path it wrote. We only require
	// that the plist file landed on disk with the correct content.
	_ = err

	want := filepath.Join(home, "Library", "LaunchAgents", SweepLabel+".plist")
	if plistPath != want {
		t.Errorf("plist path = %q, want %q", plistPath, want)
	}
	data, readErr := os.ReadFile(want)
	if readErr != nil {
		t.Fatalf("plist not written to disk: %v", readErr)
	}
	content := string(data)
	for _, frag := range []string{SweepLabel, binPath, "<string>sweep</string>", "<integer>14400</integer>"} {
		if !strings.Contains(content, frag) {
			t.Errorf("written plist missing %q", frag)
		}
	}
}
