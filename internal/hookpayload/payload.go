// Package hookpayload parses the JSON object Claude Code passes to a hook on stdin.
package hookpayload

import (
	"encoding/json"
	"io"
)

type Payload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	HookEventName  string `json:"hook_event_name"`

	// SessionStart-only.
	Model  string `json:"model,omitempty"`
	Source string `json:"source,omitempty"`

	// UserPromptSubmit-only.
	Prompt string `json:"prompt,omitempty"`

	// Stop-only.
	LastAssistantMessage string `json:"last_assistant_message,omitempty"`
	StopHookActive       bool   `json:"stop_hook_active,omitempty"`

	// PreToolUse / PostToolUse.
	ToolName     string          `json:"tool_name,omitempty"`
	ToolInput    json.RawMessage `json:"tool_input,omitempty"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	DurationMs   int             `json:"duration_ms,omitempty"`

	// PostToolBatch-only. Each call's tool_response is a string here (vs.
	// PostToolUse where it can be structured for some tools).
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse string          `json:"tool_response"`
	ToolUseID    string          `json:"tool_use_id"`
}

func Parse(r io.Reader) (*Payload, error) {
	var p Payload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// FilePathFromInput pulls a `file_path` field out of the structured tool_input
// for tools that touch files (Edit/Write/MultiEdit/NotebookEdit). Returns ""
// when the tool doesn't carry such a field.
func FilePathFromInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var v struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	_ = json.Unmarshal(input, &v)
	if v.FilePath != "" {
		return v.FilePath
	}
	return v.NotebookPath
}
