// Package plfignore parses a `.promptcellarignore` file and matches its
// patterns against a prompt's text. Per spec §4:
//
//   - One pattern per line. # comments and blank lines ignored.
//   - "id: <name>" line preceding a pattern names that pattern. Name appears
//     in `excluded.pattern_id` for the matching record.
//   - Patterns are POSIX EREs, applied case-insensitively by default.
//
// We use Go's regexp package, which implements RE2 — a strict subset of POSIX
// ERE plus standard extensions. For the kinds of secret-shape patterns the
// spec's examples use (alternation, character classes, anchors, repetition)
// the dialects are interchangeable.
package plfignore

import (
	"bufio"
	"errors"
	"os"
	"regexp"
	"strings"
)

type Pattern struct {
	ID    string
	Regex *regexp.Regexp
}

type Matcher struct {
	Patterns []Pattern
}

// Load parses the file at path. A non-existent file returns an empty matcher
// (so capture proceeds). Malformed pattern lines are skipped silently.
func Load(path string) (*Matcher, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Matcher{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var m Matcher
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
		m.Patterns = append(m.Patterns, Pattern{ID: nextID, Regex: re})
		nextID = ""
	}
	return &m, nil
}

// Match returns the first matching pattern's ID (or "" for an unnamed match)
// and ok=true on a match, false otherwise.
func (m *Matcher) Match(text string) (id string, ok bool) {
	if m == nil {
		return "", false
	}
	for _, p := range m.Patterns {
		if p.Regex.MatchString(text) {
			return p.ID, true
		}
	}
	return "", false
}
