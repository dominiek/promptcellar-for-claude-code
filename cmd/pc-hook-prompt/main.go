// Command pc-hook-prompt is the UserPromptSubmit hook entry point.
//
// Responsibilities:
//   - If a prior pending prompt exists in state with stop_seen=false, the user
//     has preempted (submitted a new prompt before the agent's Stop fired) —
//     flush the prior as outcome.status="interrupted".
//   - Match the new prompt against `.promptcellarignore`. If matched, write
//     an `excluded` stub immediately and return without buffering.
//   - Otherwise capture a fresh git snapshot and buffer the new prompt.
//   - Always print "{}" and exit 0.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"promptcellar/internal/capture"
	"promptcellar/internal/config"
	"promptcellar/internal/gitsnap"
	"promptcellar/internal/hookpayload"
	"promptcellar/internal/plf"
	"promptcellar/internal/plfignore"
	"promptcellar/internal/toolinfo"
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
	if state == nil {
		state = &capture.State{
			SessionID:        p.SessionID,
			SessionStartedAt: time.Now().UTC(),
			AuthorEmail:      gitsnap.ConfigEmail(p.Cwd),
			AuthorName:       gitsnap.ConfigName(p.Cwd),
			AuthorSigningKey: gitsnap.ConfigSigningKey(p.Cwd),
			ToolVersion:      toolinfo.Version(),
			Model:            p.Model,
		}
	}

	if state.Pending != nil {
		_ = capture.Flush(p.Cwd, capture.PromptsRoot(p.Cwd), state, p.TranscriptPath)
	}

	matcher, _ := plfignore.Load(filepath.Join(p.Cwd, ".promptcellarignore"))
	if patternID, matched := matcher.Match(p.Prompt); matched {
		_ = capture.WriteExcludedStub(p.Cwd, capture.PromptsRoot(p.Cwd), state, "matched .promptcellarignore", patternID)
		return
	}

	snap := gitsnap.Read(p.Cwd)
	state.Pending = &capture.Pending{
		ID:            plf.NewID(),
		Prompt:        p.Prompt,
		SubmittedAt:   time.Now().UTC(),
		GitBranch:     snap.Branch,
		GitHeadCommit: snap.HeadCommit,
		GitDirty:      snap.Dirty,
	}
	_ = capture.Save(p.Cwd, state)
}

func ack() { fmt.Print("{}") }

func recoverPanic() {
	if r := recover(); r != nil {
		_ = r
	}
}
