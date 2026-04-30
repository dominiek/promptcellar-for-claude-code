// Command pc-hook-tool is invoked for both PostToolUse and PostToolBatch.
//
//   - PostToolUse: only fires when a tool succeeds. We read the file_path
//     from tool_input for Edit/Write/MultiEdit/NotebookEdit and append it to
//     state.Pending.FilesTouched.
//   - PostToolBatch: fires once per agent turn, including for tools that
//     errored. We scan tool_response strings for error patterns and set
//     state.Pending.ToolErrored if any look like errors. PostToolUse alone
//     can't tell us about errors because failed tools skip it (M0 finding).
//
// Always exits 0.
package main

import (
	"fmt"
	"os"
	"strings"

	"promptcellar/internal/capture"
	"promptcellar/internal/config"
	"promptcellar/internal/hookpayload"
)

func main() {
	defer ack()
	defer recoverPanic()

	p, err := hookpayload.Parse(os.Stdin)
	if err != nil {
		return
	}
	if p.Cwd == "" || p.SessionID == "" {
		return
	}
	if !config.IsEnabled(p.Cwd) {
		return
	}

	state, _ := capture.Load(p.Cwd, p.SessionID)
	if state == nil || state.Pending == nil {
		return
	}

	switch p.HookEventName {
	case "PostToolUse":
		if path := fileTouchedBy(p.ToolName, p.ToolInput); path != "" {
			state.Pending.FilesTouched = appendUnique(state.Pending.FilesTouched, path)
		}
	case "PostToolBatch":
		for _, tc := range p.ToolCalls {
			if looksErrored(tc.ToolResponse) {
				state.Pending.ToolErrored = true
			}
		}
	default:
		return
	}

	_ = capture.Save(p.Cwd, state)
}

func fileTouchedBy(toolName string, input []byte) string {
	switch toolName {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return hookpayload.FilePathFromInput(input)
	}
	return ""
}

// looksErrored is a heuristic for the PostToolBatch tool_response strings.
// PostToolUse skips tool errors so the batch's response is the only signal.
//
// We err toward false positives being acceptable for now — `outcome.status`
// = "errored" is a hint, not a hard claim, and downstream consumers can dig
// into prompt + summary for details.
func looksErrored(resp string) bool {
	if resp == "" {
		return false
	}
	lower := strings.ToLower(resp)
	patterns := []string{
		"error:",
		"error ",
		"does not exist",
		"permission denied",
		"command not found",
		"no such file or directory",
		"failed to ",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func ack() { fmt.Print("{}") }

func recoverPanic() {
	if r := recover(); r != nil {
		_ = r
	}
}
