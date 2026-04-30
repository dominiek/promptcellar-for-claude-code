package plf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PathFor returns the canonical .prompts/YYYY/MM/DD/<session>.jsonl path for
// a given session start time (UTC) and session id.
//
// The hour bucket was dropped after v0.3.0 — empirically session counts per
// hour are low and the extra directory level mostly produced one-file
// directories. A session that crosses a day boundary keeps writing to its
// start-day file.
func PathFor(promptsRoot string, sessionStart time.Time, sessionID string) string {
	t := sessionStart.UTC()
	return filepath.Join(
		promptsRoot,
		fmt.Sprintf("%04d", t.Year()),
		fmt.Sprintf("%02d", int(t.Month())),
		fmt.Sprintf("%02d", t.Day()),
		sessionID+".jsonl",
	)
}

// AppendRecord serialises rec and atomically appends it as one line to path.
// Creates parent dirs as needed. Single-writer per file (one CC session = one
// file = one process), so no locking. The single write() of a marshaled record
// plus newline is atomic on POSIX for sizes < PIPE_BUF (4096); records will be
// well below that for v1 fields.
func AppendRecord(path string, rec *Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}
