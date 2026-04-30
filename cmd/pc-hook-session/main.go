// Command pc-hook-session is the SessionStart hook entry point.
//
// Responsibilities:
//   - Decide whether capture is enabled for this cwd; if not, exit 0.
//   - For each "orphan" state file (other session ids), flush its pending
//     prompt and remove the file.
//   - Initialise (or refresh, on source:"resume") this session's state with
//     git author, model, tool version. Preserve session_started_at across
//     resumes so the .prompts/ bucket stays stable.
//   - Always print "{}" and exit 0 — never block the user.
package main

import (
	"fmt"
	"os"
	"time"

	"promptcellar/internal/capture"
	"promptcellar/internal/config"
	"promptcellar/internal/gitsnap"
	"promptcellar/internal/hookpayload"
	"promptcellar/internal/toolinfo"
)

func main() {
	defer ack()
	defer recoverPanic()

	p, err := hookpayload.Parse(os.Stdin)
	if err != nil {
		return
	}
	cwd := p.Cwd
	if cwd == "" {
		return
	}
	if !config.IsEnabled(cwd) {
		return
	}

	flushOrphans(cwd, p.SessionID)

	existing, _ := capture.Load(cwd, p.SessionID)

	state := &capture.State{
		SessionID:        p.SessionID,
		SessionStartedAt: time.Now().UTC(),
		Model:            p.Model,
		ToolVersion:      toolinfo.Version(),
		AuthorEmail:      gitsnap.ConfigEmail(cwd),
		AuthorName:       gitsnap.ConfigName(cwd),
		AuthorSigningKey: gitsnap.ConfigSigningKey(cwd),
	}
	if existing != nil {
		state.SessionStartedAt = existing.SessionStartedAt
		state.LastPromptID = existing.LastPromptID
		if state.Model == "" {
			state.Model = existing.Model
		}
		if existing.Pending != nil {
			state.Pending = existing.Pending
		}
	}

	_ = capture.Save(cwd, state)
}

func flushOrphans(cwd, currentSessionID string) {
	others, err := capture.ListOtherSessions(cwd, currentSessionID)
	if err != nil {
		return
	}
	promptsRoot := capture.PromptsRoot(cwd)
	for _, sid := range others {
		s, err := capture.Load(cwd, sid)
		if err != nil || s == nil {
			_ = capture.Delete(cwd, sid)
			continue
		}
		if s.Pending != nil {
			_ = capture.Flush(cwd, promptsRoot, s, "")
		}
		_ = capture.Delete(cwd, sid)
	}
}

func ack() { fmt.Print("{}") }

func recoverPanic() {
	if r := recover(); r != nil {
		_ = r
	}
}
