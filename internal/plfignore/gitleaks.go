package plfignore

import (
	_ "embed"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// gitleaks.toml is the upstream gitleaks default-rules catalog (MIT, see
// vendor/gitleaks/LICENSE). 222 rules covering AWS / GCP / Azure / GitHub /
// GitLab / Anthropic / OpenAI / Stripe / Slack / Discord / Twilio / SendGrid
// / Mailgun / npm / PyPI / JWT / PEM private keys / DB URLs / etc.
//
// We embed the TOML at build time and parse it into compiled rules at the
// first call to gitleaksRules(). The parsed slice is memoised for the
// process lifetime.
//
//go:embed vendor/gitleaks/gitleaks.toml
var gitleaksTOML []byte

// gitleaksTOMLSchema is what we deserialise from the TOML. Mirrors the gitleaks
// schema, with the file/path concerns omitted — Promptcellar runs against
// prompt text, never file contents, so `paths`-based filters are inapplicable.
type gitleaksTOMLSchema struct {
	Title     string                  `toml:"title"`
	Allowlist gitleaksGlobalAllowlist `toml:"allowlist"`
	Rules     []gitleaksTOMLRule      `toml:"rules"`
}

type gitleaksGlobalAllowlist struct {
	Description string   `toml:"description"`
	Regexes     []string `toml:"regexes"`
	StopWords   []string `toml:"stopwords"`
	// Paths is intentionally ignored.
}

type gitleaksTOMLRule struct {
	ID          string                       `toml:"id"`
	Description string                       `toml:"description"`
	Regex       string                       `toml:"regex"`
	Entropy     float64                      `toml:"entropy"`
	Keywords    []string                     `toml:"keywords"`
	Allowlists  []gitleaksTOMLRuleAllowlist  `toml:"allowlists"`
}

type gitleaksTOMLRuleAllowlist struct {
	Description string   `toml:"description"`
	Regexes     []string `toml:"regexes"`
	StopWords   []string `toml:"stopwords"`
	// RegexTarget is "match" or "secret"; we currently treat both as "match"
	// because Promptcellar only stores a single match span.
	RegexTarget string `toml:"regexTarget"`
}

// compiledGitleaksRule is the runtime form: regex compiled, allowlists
// flattened to a single combined regex, keyword pre-filter ready for
// substring matching.
type compiledGitleaksRule struct {
	ID          string
	Description string
	Regex       *regexp.Regexp
	Entropy     float64
	Keywords    []string
	Allowlist   []*regexp.Regexp
	AllowlistSW []string
}

// compiledGitleaks is the lazily-loaded singleton.
type compiledGitleaks struct {
	GlobalAllowRegex []*regexp.Regexp
	GlobalAllowSW    []string
	Rules            []compiledGitleaksRule
}

var compiledGitleaksOnce struct {
	c   *compiledGitleaks
	err error
}

// gitleaksRules parses + compiles the embedded gitleaks TOML on first call.
// Subsequent calls return the same slice. A failure to parse is logged once
// and returns nil — the matcher continues with PII-only and user-file rules.
func gitleaksRules() *compiledGitleaks {
	if compiledGitleaksOnce.c != nil || compiledGitleaksOnce.err != nil {
		return compiledGitleaksOnce.c
	}
	c, err := compileGitleaks(gitleaksTOML)
	compiledGitleaksOnce.c = c
	compiledGitleaksOnce.err = err
	return c
}

func compileGitleaks(raw []byte) (*compiledGitleaks, error) {
	var schema gitleaksTOMLSchema
	if err := toml.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("parse gitleaks.toml: %w", err)
	}

	out := &compiledGitleaks{
		GlobalAllowSW: schema.Allowlist.StopWords,
	}
	for _, r := range schema.Allowlist.Regexes {
		re, err := regexp.Compile(r)
		if err != nil {
			continue // skip malformed entries silently
		}
		out.GlobalAllowRegex = append(out.GlobalAllowRegex, re)
	}

	for _, r := range schema.Rules {
		if r.Regex == "" {
			continue
		}
		re, err := regexp.Compile(r.Regex)
		if err != nil {
			continue
		}
		cr := compiledGitleaksRule{
			ID:          r.ID,
			Description: r.Description,
			Regex:       re,
			Entropy:     r.Entropy,
			Keywords:    r.Keywords,
		}
		for _, alw := range r.Allowlists {
			for _, ar := range alw.Regexes {
				are, err := regexp.Compile(ar)
				if err == nil {
					cr.Allowlist = append(cr.Allowlist, are)
				}
			}
			cr.AllowlistSW = append(cr.AllowlistSW, alw.StopWords...)
		}
		out.Rules = append(out.Rules, cr)
	}
	return out, nil
}

// matchGitleaks returns the first rule that fires against text, applying the
// rule's keyword pre-filter, regex match, per-rule allowlist, global
// allowlist, and entropy threshold in that order. Returns "" / false if no
// rule matches.
func matchGitleaks(c *compiledGitleaks, text string) (id string, ok bool) {
	if c == nil {
		return "", false
	}
	lowered := "" // lazy — only computed if any rule actually has keywords
	for _, rule := range c.Rules {
		if len(rule.Keywords) > 0 {
			if lowered == "" {
				lowered = strings.ToLower(text)
			}
			hit := false
			for _, kw := range rule.Keywords {
				if strings.Contains(lowered, strings.ToLower(kw)) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		match := rule.Regex.FindString(text)
		if match == "" {
			continue
		}
		if matchesAny(rule.Allowlist, match) || containsAny(match, rule.AllowlistSW) {
			continue
		}
		if matchesAny(c.GlobalAllowRegex, match) || containsAny(match, c.GlobalAllowSW) {
			continue
		}
		if rule.Entropy > 0 && shannonEntropy(match) < rule.Entropy {
			continue
		}
		return rule.ID, true
	}
	return "", false
}

func matchesAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

func containsAny(s string, needles []string) bool {
	if len(needles) == 0 {
		return false
	}
	lower := strings.ToLower(s)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// shannonEntropy returns the Shannon entropy (bits per character) of s.
// Used to filter false-positive regex hits on low-entropy strings like
// "AAAAAAAA" or "0123456789012345".
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int, 64)
	for _, r := range s {
		counts[r]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}
