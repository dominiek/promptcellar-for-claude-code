// Package toolinfo detects the running Claude Code version from environment
// variables that CC reliably sets in hook subprocesses.
package toolinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// Version returns the Claude Code version. Detection order:
//
//  1. AI_AGENT env var, e.g. "claude-code/2.1.123/harness" → "2.1.123".
//     Empirically present in every hook fire we've observed in CC 2.1.x.
//  2. CLAUDE_CODE_EXECPATH env var, e.g. ".../versions/2.1.123" → "2.1.123".
//     Not actually set in hook subprocesses by CC 2.1.x — kept as a defensive
//     fallback in case future versions expose it.
//  3. "unknown" — last resort. Satisfies PLF schema's minLength:1 requirement
//     so we still emit a valid record.
func Version() string {
	if a := os.Getenv("AI_AGENT"); a != "" {
		parts := strings.Split(a, "/")
		if len(parts) >= 2 && parts[1] != "" {
			return parts[1]
		}
	}
	if p := os.Getenv("CLAUDE_CODE_EXECPATH"); p != "" {
		v := filepath.Base(p)
		if v != "" && v != "/" && v != "." {
			return v
		}
	}
	return "unknown"
}
