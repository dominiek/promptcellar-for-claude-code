// Package plfignore parses `.promptcellarignore` and `.promptcellarallow`
// and matches their patterns against a prompt's text. Layered with a built-in
// baseline (see baseline.go), it decides whether a prompt is captured normally
// or replaced with an `excluded` stub per spec §4.
//
// Resolution order (most authoritative first):
//
//  1. .promptcellarignore — team-authored deny rules. Always wins.
//  2. baseline (built-in)  — covers well-known secret shapes.
//                            Overridden by .promptcellarallow when both fire.
//  3. .promptcellarallow   — team-authored exception list. Only narrows the
//                            baseline; never weakens .promptcellarignore.
//
// Pattern syntax (same in both files):
//
//   - One pattern per line. # comments and blank lines ignored.
//   - "id: <name>" line preceding a pattern names it. Name appears in
//     `excluded.pattern_id` for matches against the ignore file.
//   - Patterns are POSIX EREs, applied case-insensitively by default.
//
// We use Go's regexp/RE2, which is a strict subset of POSIX ERE plus standard
// extensions. For typical secret-shape patterns (alternation, character
// classes, anchors, repetition) the dialects are interchangeable.
package plfignore

import (
	"bufio"
	"errors"
	"os"
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
	SourceBaseline MatchSource = "baseline"
)

// Result of evaluating a prompt against the layered matcher.
//
//   - Excluded: true if the prompt should be turned into an `excluded` stub.
//   - PatternID: the matched pattern's id (populated only when Excluded).
//   - Source: whether the match came from .promptcellarignore or the baseline.
type Result struct {
	Excluded  bool
	PatternID string
	Source    MatchSource
}

// Matcher composes the three layers and evaluates prompts against them.
type Matcher struct {
	ignore   []Pattern
	baseline []Pattern
	allow    []Pattern
}

// LoadAll builds a layered Matcher for the given repo cwd.
//
// The baseline is always loaded (compiled into the binary). The ignore and
// allow files are loaded if they exist; missing files yield zero patterns at
// that layer.
func LoadAll(cwd string) (*Matcher, error) {
	ignore, err := loadFile(cwd + "/" + IgnoreFilename)
	if err != nil {
		return nil, err
	}
	allow, err := loadFile(cwd + "/" + AllowFilename)
	if err != nil {
		return nil, err
	}
	return &Matcher{
		ignore:   ignore,
		baseline: compileBaseline(),
		allow:    allow,
	}, nil
}

// Match evaluates a prompt against the three layers and returns a Result.
// On no match, Result.Excluded is false; the caller captures normally.
func (m *Matcher) Match(text string) Result {
	if m == nil {
		return Result{}
	}
	if id, ok := matchAny(m.ignore, text); ok {
		return Result{Excluded: true, PatternID: id, Source: SourceIgnore}
	}
	if id, ok := matchAny(m.baseline, text); ok {
		if _, allowed := matchAny(m.allow, text); allowed {
			return Result{}
		}
		return Result{Excluded: true, PatternID: id, Source: SourceBaseline}
	}
	return Result{}
}

// Counts returns the active pattern counts per layer. Used by doctor / status.
func (m *Matcher) Counts() (ignore, baseline, allow int) {
	if m == nil {
		return 0, 0, 0
	}
	return len(m.ignore), len(m.baseline), len(m.allow)
}

func matchAny(patterns []Pattern, text string) (string, bool) {
	for _, p := range patterns {
		if p.Regex.MatchString(text) {
			return p.ID, true
		}
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

// compileBaseline turns baselinePatterns (raw strings) into compiled Patterns.
// Compiled lazily-once at process start (Matcher creation); each invocation of
// LoadAll currently recompiles. That's a few hundred microseconds; if it ever
// matters we'll memoise.
func compileBaseline() []Pattern {
	out := make([]Pattern, 0, len(baselinePatterns))
	for _, p := range baselinePatterns {
		// Baseline patterns may already include their own (?i) prefix where
		// case-sensitivity matters (e.g. AWS access keys are all-uppercase).
		// Default still adds (?i) so generic patterns work as documented.
		expr := p.Regex
		if !strings.HasPrefix(expr, "(?i)") && !strings.HasPrefix(expr, "(?-i)") {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			// A malformed baseline pattern is a programmer error — skip it
			// quietly rather than panicking from inside a hook subprocess.
			continue
		}
		out = append(out, Pattern{ID: p.ID, Regex: re})
	}
	return out
}

// ─── Backwards-compat wrappers ───────────────────────────────────────────────
// The single-file Load() API was used in M2 before the baseline + allow layers
// existed. Kept around for callers that only want the team's deny list and
// nothing else (none currently in-tree, but we preserve the export for any
// out-of-tree tooling).

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
