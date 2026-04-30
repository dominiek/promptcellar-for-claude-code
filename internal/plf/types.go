// Package plf defines the on-disk Promptcellar Logging Format v1 record types.
//
// Field order in struct declarations matches the order required by the spec
// for diff-friendly output. encoding/json marshals fields in declaration order.
package plf

import (
	"crypto/rand"
	"fmt"
)

const Version = "plf-1"

type Record struct {
	Version     string       `json:"version"`
	ID          string       `json:"id"`
	SessionID   string       `json:"session_id"`
	Timestamp   string       `json:"timestamp"`
	Author      Author       `json:"author"`
	Tool        Tool         `json:"tool"`
	Model       Model        `json:"model"`
	Prompt      string       `json:"prompt,omitempty"`
	Git         *Git         `json:"git,omitempty"`
	Parent      *Parent      `json:"parent,omitempty"`
	Outcome     *Outcome     `json:"outcome,omitempty"`
	Enrichments *Enrichments `json:"enrichments,omitempty"`
	Excluded    *Excluded    `json:"excluded,omitempty"`
}

// Excluded marks a stub record written when the prompt matched a
// .promptcellarignore pattern. Per spec §3.9, when Excluded is set, prompt /
// git / parent / outcome / enrichments MUST be absent.
type Excluded struct {
	Reason    string `json:"reason"`
	PatternID string `json:"pattern_id,omitempty"`
}

type Enrichments struct {
	Tokens     *Tokens `json:"tokens,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`
	DurationMs int64   `json:"duration_ms,omitempty"`
}

type Tokens struct {
	Input      int `json:"input,omitempty"`
	Output     int `json:"output,omitempty"`
	CacheRead  int `json:"cache_read,omitempty"`
	CacheWrite int `json:"cache_write,omitempty"`
}

type Author struct {
	Email string  `json:"email"`
	Name  string  `json:"name"`
	ID    *string `json:"id,omitempty"`
}

type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Model struct {
	Provider string  `json:"provider"`
	Name     string  `json:"name"`
	Version  *string `json:"version,omitempty"`
}

type Git struct {
	Branch     string `json:"branch,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	Dirty      bool   `json:"dirty"`
}

type Parent struct {
	PromptID string `json:"prompt_id"`
}

type Outcome struct {
	Summary      string   `json:"summary,omitempty"`
	FilesTouched []string `json:"files_touched,omitempty"`
	Commits      []string `json:"commits,omitempty"`
	Status       string   `json:"status,omitempty"`
}

const (
	StatusCompleted   = "completed"
	StatusErrored     = "errored"
	StatusInterrupted = "interrupted"
	StatusUnknown     = "unknown"
)

// NewID returns a new UUIDv4 string. No external deps.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
