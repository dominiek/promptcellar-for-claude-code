// Package capture owns the per-session state file under
// <cwd>/.promptcellar/state/<session-id>.json — gitignored, single-writer.
//
// The state file accumulates everything we need to emit a PLF record at flush
// time. Flush happens at the next hook fire after Stop (or at next session's
// SessionStart for orphan recovery), because the assistant record we need for
// model/token enrichment is not yet on disk at Stop.
package capture

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	StateDir   = ".promptcellar/state"
	PromptsDir = ".prompts"
)

type State struct {
	SessionID        string    `json:"session_id"`
	SessionStartedAt time.Time `json:"session_started_at"`
	Model            string    `json:"model,omitempty"`
	ToolVersion      string    `json:"tool_version"`
	AuthorEmail      string    `json:"author_email"`
	AuthorName       string    `json:"author_name"`
	AuthorSigningKey string    `json:"author_signingkey,omitempty"`
	LastPromptID     string    `json:"last_prompt_id,omitempty"`
	Pending          *Pending  `json:"pending,omitempty"`
}

type Pending struct {
	ID                   string     `json:"id"`
	Prompt               string     `json:"prompt"`
	SubmittedAt          time.Time  `json:"submitted_at"`
	GitBranch            string     `json:"git_branch,omitempty"`
	GitHeadCommit        string     `json:"git_head_commit,omitempty"`
	GitDirty             bool       `json:"git_dirty"`
	StopSeen             bool       `json:"stop_seen"`
	LastAssistantMessage string     `json:"last_assistant_message,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	FilesTouched         []string   `json:"files_touched,omitempty"`
	ToolErrored          bool       `json:"tool_errored,omitempty"`
}

func StatePath(cwd, sessionID string) string {
	return filepath.Join(cwd, StateDir, sessionID+".json")
}

func StateRoot(cwd string) string {
	return filepath.Join(cwd, StateDir)
}

func PromptsRoot(cwd string) string {
	return filepath.Join(cwd, PromptsDir)
}

func Load(cwd, sessionID string) (*State, error) {
	data, err := os.ReadFile(StatePath(cwd, sessionID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func Save(cwd string, s *State) error {
	if err := os.MkdirAll(StateRoot(cwd), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := StatePath(cwd, s.SessionID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, StatePath(cwd, s.SessionID))
}

func Delete(cwd, sessionID string) error {
	err := os.Remove(StatePath(cwd, sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ListOtherSessions returns session ids of state files in StateDir excluding
// the current session id. Used by SessionStart for orphan recovery.
func ListOtherSessions(cwd, currentSessionID string) ([]string, error) {
	entries, err := os.ReadDir(StateRoot(cwd))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !endsWith(name, ".json") {
			continue
		}
		sid := name[:len(name)-len(".json")]
		if sid == currentSessionID {
			continue
		}
		out = append(out, sid)
	}
	return out, nil
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
