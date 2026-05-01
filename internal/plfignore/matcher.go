// Package plfignore implements Promptcellar's log-redaction matcher: it
// decides whether a prompt is captured normally or replaced with an
// `excluded` stub per spec §4.
//
// "Log redaction" is the term-of-art for stripping secrets and PII out of a
// log stream as it flows through. We borrow the framing here even though
// the substrate is structured prompt records rather than free-form log
// lines — the goals are the same: stop sensitive content from landing on
// disk in the first place.
//
// Three pattern sources, evaluated in this layered order on every prompt:
//
//  1. .promptcellarignore (team-authored deny list, committed)
//                                                  — always wins.
//  2. Built-in secret rules (vendored gitleaks default config — 222 rules,
//                            MIT, see vendor/gitleaks/) +
//     Built-in PII rules    (hand-rolled in pii.go — credit card / Luhn,
//                            IBAN / MOD-97, US SSN, email, phone)
//                                                  — overridden by allow.
//  3. .promptcellarallow (team-authored exception list, committed)
//                                                  — only narrows the
//                                                    built-in layer; never
//                                                    weakens .promptcellarignore.
//
// Pattern syntax (same in both team-authored files):
//
//   - One pattern per line. # comments and blank lines ignored.
//   - "id: <name>" on a line preceding a pattern names it. Name appears in
//     `excluded.pattern_id` for that match.
//   - Patterns are POSIX EREs (RE2-compatible Go regex), case-insensitive
//     by default.
package plfignore

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	IgnoreFilename = ".promptcellarignore"
	AllowFilename  = ".promptcellarallow"
)

type Pattern struct {
	ID    string
	Regex *regexp.Regexp
}

// MatchSource identifies which layer caused a match.
type MatchSource string

const (
	SourceIgnore   MatchSource = "ignore"
	SourceGitleaks MatchSource = "gitleaks"
	SourcePII      MatchSource = "pii"
)

// Result of evaluating a prompt against the layered matcher.
type Result struct {
	Excluded  bool
	PatternID string
	Source    MatchSource
}

// Matcher composes the four layers and evaluates prompts against them.
type Matcher struct {
	ignore   []Pattern
	gitleaks *compiledGitleaks
	pii      []piiPattern // shadows the package var so tests can swap
	allow    []Pattern
}

// LoadAll builds a layered Matcher for the given repo cwd.
//
// Built-in layers (gitleaks + PII) are always loaded — they're compiled into
// the binary. The ignore and allow files are loaded if they exist; missing
// files yield zero patterns at that layer.
func LoadAll(cwd string) (*Matcher, error) {
	ignore, err := loadFile(filepath.Join(cwd, IgnoreFilename))
	if err != nil {
		return nil, err
	}
	allow, err := loadFile(filepath.Join(cwd, AllowFilename))
	if err != nil {
		return nil, err
	}
	return &Matcher{
		ignore:   ignore,
		gitleaks: gitleaksRules(),
		pii:      piiPatterns,
		allow:    allow,
	}, nil
}

// Match evaluates a prompt against the layered matcher and returns a Result.
// On no match, Result.Excluded is false; the caller captures normally.
func (m *Matcher) Match(text string) Result {
	if m == nil {
		return Result{}
	}
	if id, ok := matchAny(m.ignore, text); ok {
		return Result{Excluded: true, PatternID: id, Source: SourceIgnore}
	}
	if id, ok := matchGitleaks(m.gitleaks, text); ok {
		if _, allowed := matchAny(m.allow, text); allowed {
			return Result{}
		}
		return Result{Excluded: true, PatternID: id, Source: SourceGitleaks}
	}
	if id, ok := matchPIIWith(m.pii, text); ok {
		if _, allowed := matchAny(m.allow, text); allowed {
			return Result{}
		}
		return Result{Excluded: true, PatternID: id, Source: SourcePII}
	}
	return Result{}
}

// Counts returns the active pattern counts per layer. Used by doctor.
func (m *Matcher) Counts() (ignore, gitleaks, pii, allow int) {
	if m == nil {
		return 0, 0, 0, 0
	}
	gleaks := 0
	if m.gitleaks != nil {
		gleaks = len(m.gitleaks.Rules)
	}
	return len(m.ignore), gleaks, len(m.pii), len(m.allow)
}

func matchAny(patterns []Pattern, text string) (string, bool) {
	for _, p := range patterns {
		if p.Regex.MatchString(text) {
			return p.ID, true
		}
	}
	return "", false
}

// matchPIIWith is matchPII but takes the slice as a parameter so the
// per-Matcher field (which can be swapped in tests) is honoured.
func matchPIIWith(patterns []piiPattern, text string) (id string, ok bool) {
	for _, p := range patterns {
		hit := p.Regex.FindString(text)
		if hit == "" {
			continue
		}
		if p.Validate != nil && !p.Validate(hit) {
			continue
		}
		return p.ID, true
	}
	return "", false
}

// loadFile parses an ignore-or-allow-shaped file. Missing file → empty slice.
func loadFile(path string) ([]Pattern, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Pattern
	var nextID string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		if strings.HasPrefix(stripped, "id:") {
			nextID = strings.TrimSpace(strings.TrimPrefix(stripped, "id:"))
			continue
		}
		re, err := regexp.Compile("(?i)" + stripped)
		if err != nil {
			nextID = ""
			continue
		}
		out = append(out, Pattern{ID: nextID, Regex: re})
		nextID = ""
	}
	return out, nil
}

// ─── Backwards-compat single-file API ────────────────────────────────────────
// Preserved for any out-of-tree consumers that imported the M2-era API.

type SingleFileMatcher struct{ Patterns []Pattern }

func Load(path string) (*SingleFileMatcher, error) {
	patterns, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	return &SingleFileMatcher{Patterns: patterns}, nil
}

func (s *SingleFileMatcher) Match(text string) (id string, ok bool) {
	if s == nil {
		return "", false
	}
	return matchAny(s.Patterns, text)
}
