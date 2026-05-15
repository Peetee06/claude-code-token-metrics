package repo

import (
	"bufio"
	"encoding/json"
	"os"
)

// FirstCwd scans a transcript JSONL file and returns the cwd value from the
// first line that carries a non-empty cwd field. Returns "" if no line has
// one. Malformed lines are skipped silently — analyze counts them separately.
func FirstCwd(transcriptPath string) (string, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Transcript lines can be large; raise the line-size ceiling to 8 MiB.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var line struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue // skip malformed line
		}
		if line.Cwd != "" {
			return line.Cwd, nil
		}
	}
	return "", sc.Err()
}
