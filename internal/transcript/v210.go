// Package transcript reads Claude Code's session transcript JSONL files.
//
// Targeted at the CC 2.1.x transcript shape documented in
// planning/HOOK_PAYLOAD_REFERENCE.md. Future versions will get sibling
// adapters and a dispatcher.
package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

type Usage struct {
	Input      int
	Output     int
	CacheRead  int
	CacheWrite int
}

type AssistantRecord struct {
	Model     string
	RequestID string
	Timestamp time.Time
	Usage     Usage
}

// jsonAssistant matches the CC 2.1.x assistant record shape — only the fields
// we care about. Unknown fields are ignored by the json decoder.
type jsonAssistant struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	RequestID string `json:"requestId"`
	Message   struct {
		Model string `json:"model"`
		Usage struct {
			Input      int `json:"input_tokens"`
			Output     int `json:"output_tokens"`
			CacheRead  int `json:"cache_read_input_tokens"`
			CacheWrite int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ReadAssistantRecords scans the transcript and returns every assistant turn.
// Errors return an empty slice rather than propagating — capture must remain
// best-effort.
func ReadAssistantRecords(transcriptPath string) []AssistantRecord {
	if transcriptPath == "" {
		return nil
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []AssistantRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var rec jsonAssistant
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Type != "assistant" {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, rec.Timestamp)
		out = append(out, AssistantRecord{
			Model:     rec.Message.Model,
			RequestID: rec.RequestID,
			Timestamp: ts,
			Usage: Usage{
				Input:      rec.Message.Usage.Input,
				Output:     rec.Message.Usage.Output,
				CacheRead:  rec.Message.Usage.CacheRead,
				CacheWrite: rec.Message.Usage.CacheWrite,
			},
		})
	}
	return out
}

// FindLatestModel returns the model name on the most recent assistant record,
// or "" if none. Used as a fallback when SessionStart's payload omits `model`
// (headless / sdk-cli / source:"resume").
func FindLatestModel(transcriptPath string) string {
	records := ReadAssistantRecords(transcriptPath)
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Model != "" {
			return records[i].Model
		}
	}
	return ""
}

// SumUsageSince sums input/output/cache tokens across assistant records whose
// timestamp is at or after `since`. Used at flush time to attribute tokens to
// the just-finished prompt.
func SumUsageSince(records []AssistantRecord, since time.Time) Usage {
	var sum Usage
	for _, r := range records {
		if r.Timestamp.IsZero() || r.Timestamp.Before(since) {
			continue
		}
		sum.Input += r.Usage.Input
		sum.Output += r.Usage.Output
		sum.CacheRead += r.Usage.CacheRead
		sum.CacheWrite += r.Usage.CacheWrite
	}
	return sum
}

// WaitForRecordSince polls the transcript until at least one assistant record
// with timestamp >= since exists, or `timeout` elapses. Returns all assistant
// records found at exit. If timeout hits, returns whatever's there.
//
// Used inside the Stop hook to absorb the small delay between Stop firing and
// CC persisting the just-finished assistant message (M0 finding).
func WaitForRecordSince(transcriptPath string, since time.Time, timeout, interval time.Duration) []AssistantRecord {
	deadline := time.Now().Add(timeout)
	for {
		records := ReadAssistantRecords(transcriptPath)
		for _, r := range records {
			if !r.Timestamp.Before(since) {
				return records
			}
		}
		if time.Now().After(deadline) {
			return records
		}
		time.Sleep(interval)
	}
}
