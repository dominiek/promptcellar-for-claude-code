// Command pc-hook-stop is the Stop hook entry point.
//
// Design: flush-at-Stop. The PLF record for the just-finished prompt is
// written to .prompts/...jsonl synchronously inside this hook so the
// repository's prompt log is always current relative to the user's commits.
//
// Before flushing we briefly poll the transcript for the just-finished
// assistant record (CC persists it asynchronously after Stop, M0 finding).
// Typical wait is sub-second; we cap it so a slow filesystem can't hold up
// the user.
package main

import (
	"fmt"
	"os"
	"time"

	"promptcellar/internal/capture"
	"promptcellar/internal/config"
	"promptcellar/internal/hookpayload"
	"promptcellar/internal/transcript"
)

const (
	transcriptWaitTotal    = 3 * time.Second
	transcriptWaitInterval = 100 * time.Millisecond
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

	state.Pending.StopSeen = true
	state.Pending.LastAssistantMessage = p.LastAssistantMessage
	completedAt := time.Now().UTC()
	state.Pending.CompletedAt = &completedAt

	// Wait briefly for CC to finish persisting the assistant record so we can
	// enrich with token usage. If timeout hits we flush anyway — `outcome.*`
	// is fully captured at this point regardless.
	transcript.WaitForRecordSince(p.TranscriptPath, state.Pending.SubmittedAt, transcriptWaitTotal, transcriptWaitInterval)

	_ = capture.Flush(p.Cwd, capture.PromptsRoot(p.Cwd), state, p.TranscriptPath)
}

func ack() { fmt.Print("{}") }

func recoverPanic() {
	if r := recover(); r != nil {
		_ = r
	}
}
