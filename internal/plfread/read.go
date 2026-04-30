// Package plfread walks a `.prompts/` data store and returns its records.
// Used by the CLI (`pc-cli log`) and the MCP retrieval server.
package plfread

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"promptcellar/internal/plf"
)

// ReadAll walks every JSONL file under promptsRoot and returns the parsed
// records, sorted by timestamp descending (newest first). Malformed lines are
// skipped silently. Missing root → empty slice.
func ReadAll(promptsRoot string) ([]plf.Record, error) {
	var out []plf.Record
	err := filepath.WalkDir(promptsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 32*1024*1024)
		for sc.Scan() {
			var rec plf.Record
			if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
				continue
			}
			out = append(out, rec)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

// Search returns records whose prompt text contains q (case-insensitive).
// Excluded stubs (no prompt) are skipped.
func Search(records []plf.Record, q string) []plf.Record {
	q = strings.ToLower(q)
	var out []plf.Record
	for _, r := range records {
		if r.Prompt == "" {
			continue
		}
		if strings.Contains(strings.ToLower(r.Prompt), q) {
			out = append(out, r)
		}
	}
	return out
}

// Touched returns records whose outcome.files_touched contains path (substring
// match against repo-relative entries).
func Touched(records []plf.Record, path string) []plf.Record {
	var out []plf.Record
	for _, r := range records {
		if r.Outcome == nil {
			continue
		}
		for _, f := range r.Outcome.FilesTouched {
			if strings.Contains(f, path) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Session returns all records belonging to a session id.
func Session(records []plf.Record, sessionID string) []plf.Record {
	var out []plf.Record
	for _, r := range records {
		if r.SessionID == sessionID {
			out = append(out, r)
		}
	}
	// Within a session we want chronological order.
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}
