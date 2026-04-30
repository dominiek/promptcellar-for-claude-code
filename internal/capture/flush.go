package capture

import (
	"path/filepath"
	"strings"
	"time"

	"promptcellar/internal/gitsnap"
	"promptcellar/internal/plf"
	"promptcellar/internal/pricing"
	"promptcellar/internal/transcript"
)

const summaryMaxChars = 500

const fallbackModel = "claude-opus-4-7"

// BuildRecord converts a populated state (with non-nil Pending) into a PLF
// record. Status is decided by Pending.StopSeen / Pending.ToolErrored.
//
// transcriptPath is consulted as a fallback for model.name and to compute
// enrichments.tokens.* / cost_usd. cwd is used to resolve files_touched paths
// to repo-relative form and to scan `git log --since` for outcome.commits.
func BuildRecord(s *State, cwd, transcriptPath string) *plf.Record {
	p := s.Pending
	model := resolveModel(s.Model, transcriptPath)
	rec := &plf.Record{
		Version:   plf.Version,
		ID:        p.ID,
		SessionID: s.SessionID,
		Timestamp: p.SubmittedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Author: plf.Author{
			Email: s.AuthorEmail,
			Name:  s.AuthorName,
		},
		Tool: plf.Tool{
			Name:    "claude-code",
			Version: s.ToolVersion,
		},
		Model: plf.Model{
			Provider: "anthropic",
			Name:     model,
		},
		Prompt: p.Prompt,
	}
	if s.AuthorSigningKey != "" {
		key := s.AuthorSigningKey
		rec.Author.ID = &key
	}
	if p.GitBranch != "" || p.GitHeadCommit != "" {
		rec.Git = &plf.Git{
			Branch:     p.GitBranch,
			HeadCommit: p.GitHeadCommit,
			Dirty:      p.GitDirty,
		}
	}
	if s.LastPromptID != "" {
		rec.Parent = &plf.Parent{PromptID: s.LastPromptID}
	}

	rec.Outcome = buildOutcome(p, cwd)
	if p.StopSeen {
		rec.Enrichments = buildEnrichments(p, model, transcriptPath)
	}

	return rec
}

func buildOutcome(p *Pending, cwd string) *plf.Outcome {
	o := &plf.Outcome{}
	switch {
	case !p.StopSeen:
		o.Status = plf.StatusInterrupted
	case p.ToolErrored:
		o.Status = plf.StatusErrored
		o.Summary = truncate(p.LastAssistantMessage, summaryMaxChars)
	default:
		o.Status = plf.StatusCompleted
		o.Summary = truncate(p.LastAssistantMessage, summaryMaxChars)
	}

	if len(p.FilesTouched) > 0 {
		o.FilesTouched = relPaths(cwd, p.FilesTouched)
	}
	if commits := gitsnap.CommitsSinceSha(cwd, p.GitHeadCommit); len(commits) > 0 {
		o.Commits = commits
	}
	return o
}

func buildEnrichments(p *Pending, model, transcriptPath string) *plf.Enrichments {
	e := &plf.Enrichments{}
	if p.CompletedAt != nil && !p.SubmittedAt.IsZero() {
		e.DurationMs = p.CompletedAt.Sub(p.SubmittedAt).Milliseconds()
	}
	records := transcript.ReadAssistantRecords(transcriptPath)
	usage := transcript.SumUsageSince(records, p.SubmittedAt)
	if usage.Input > 0 || usage.Output > 0 || usage.CacheRead > 0 || usage.CacheWrite > 0 {
		e.Tokens = &plf.Tokens{
			Input:      usage.Input,
			Output:     usage.Output,
			CacheRead:  usage.CacheRead,
			CacheWrite: usage.CacheWrite,
		}
		cost := pricing.ComputeUSD(model, usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite)
		if cost > 0 {
			// round to 6 decimals — sub-cent precision is plenty for analytics.
			e.CostUSD = roundTo(cost, 6)
		}
	}
	if e.Tokens == nil && e.DurationMs == 0 && e.CostUSD == 0 {
		return nil
	}
	return e
}

// Flush builds the PLF record from state.Pending, appends to JSONL, and clears
// the pending slot. transcriptPath is consulted for model + enrichments.
func Flush(cwd, promptsRoot string, s *State, transcriptPath string) error {
	if s == nil || s.Pending == nil {
		return nil
	}
	rec := BuildRecord(s, cwd, transcriptPath)
	dest := plf.PathFor(promptsRoot, s.SessionStartedAt, s.SessionID)
	if err := plf.AppendRecord(dest, rec); err != nil {
		return err
	}
	s.LastPromptID = s.Pending.ID
	s.Pending = nil
	return Save(cwd, s)
}

// WriteExcludedStub appends an `excluded` record to the session's JSONL when
// a prompt matched a `.promptcellarignore` pattern. Per spec §3.9 the stub
// preserves the timeline so consumers can see capture was skipped, without
// retaining the prompt text or any of the captured-record extras.
//
// Callers must NOT also buffer a Pending for this prompt — the stub is the
// final record for it.
func WriteExcludedStub(cwd, promptsRoot string, s *State, reason, patternID string) error {
	if s == nil {
		return nil
	}
	id := plf.NewID()
	now := time.Now().UTC()
	rec := &plf.Record{
		Version:   plf.Version,
		ID:        id,
		SessionID: s.SessionID,
		Timestamp: now.Format("2006-01-02T15:04:05.000Z07:00"),
		Author: plf.Author{
			Email: s.AuthorEmail,
			Name:  s.AuthorName,
		},
		Tool: plf.Tool{
			Name:    "claude-code",
			Version: s.ToolVersion,
		},
		Model: plf.Model{
			Provider: "anthropic",
			Name:     resolveModel(s.Model, ""),
		},
		Excluded: &plf.Excluded{
			Reason:    reason,
			PatternID: patternID,
		},
	}
	dest := plf.PathFor(promptsRoot, s.SessionStartedAt, s.SessionID)
	if err := plf.AppendRecord(dest, rec); err != nil {
		return err
	}
	s.LastPromptID = id
	return Save(cwd, s)
}

func resolveModel(stateModel, transcriptPath string) string {
	if stateModel != "" {
		return stateModel
	}
	if m := transcript.FindLatestModel(transcriptPath); m != "" {
		return m
	}
	return fallbackModel
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return strings.TrimRight(s[:cut], " \n\t")
}

func relPaths(cwd string, abs []string) []string {
	out := make([]string, 0, len(abs))
	seen := map[string]bool{}
	for _, p := range abs {
		rel := p
		if r, err := filepath.Rel(cwd, p); err == nil {
			rel = r
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	return out
}

func roundTo(f float64, places int) float64 {
	mul := 1.0
	for i := 0; i < places; i++ {
		mul *= 10
	}
	return float64(int64(f*mul+0.5)) / mul
}

