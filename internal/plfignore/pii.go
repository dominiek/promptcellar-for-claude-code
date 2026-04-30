package plfignore

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// PII patterns. Hand-rolled in Go because gitleaks ships zero PII rules
// (it's a secret-scanner) and Microsoft Presidio's catalog uses Python
// lookbehind syntax that Go's RE2 can't compile without rewriting.
//
// Each pattern has an optional `Validate` callback that runs after the
// regex matches. It exists so we can reject false positives that pass the
// regex but fail the underlying algorithm — Luhn for credit cards, MOD-97
// for IBANs. Without checksums, generic 16-digit / IBAN-shaped regexes
// false-positive aggressively on order numbers, transaction IDs, etc.
//
// IDs match `[A-Za-z0-9_-]+` per PLF schema for `excluded.pattern_id`.

type piiPattern struct {
	ID          string
	Description string
	Regex       *regexp.Regexp
	// Validate runs against the matched substring. nil means "regex match
	// alone is enough." Returning false means the match is rejected (allowed
	// through as a non-match).
	Validate func(string) bool
}

var piiPatterns = []piiPattern{
	{
		ID:          "credit-card",
		Description: "Major-issuer credit card number with valid Luhn check (Visa/MC/Amex/Discover/JCB/Diners)",
		Regex: regexp.MustCompile(
			`\b(?:` +
				`4\d{12}(?:\d{3})?` + // Visa 13 or 16
				`|5[1-5]\d{14}` + // MC 16
				`|2(?:2(?:2[1-9]|[3-9]\d)|[3-6]\d{2}|7(?:[01]\d|20))\d{12}` + // MC 2-series
				`|3[47]\d{13}` + // Amex 15
				`|6(?:011|5\d{2})\d{12}` + // Discover
				`|(?:2131|1800)\d{11}|35\d{14}` + // JCB
				`|3(?:0[0-5]|[68]\d)\d{11}` + // Diners
				`)\b`,
		),
		Validate: func(s string) bool { return luhnValid(stripNonDigits(s)) },
	},
	{
		ID:          "iban",
		Description: "IBAN with valid MOD-97 checksum",
		// Two-letter country code, two check digits, 11–30 alphanumerics.
		Regex:    regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`),
		Validate: ibanValid,
	},
	{
		ID:          "us-ssn",
		Description: "US Social Security Number (XXX-XX-XXXX, SSA-rule validated)",
		Regex:       regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		// SSA invalidity rules (regex would need lookarounds RE2 doesn't
		// support, so we validate after match): area must not be 000 / 666
		// / 9XX; group must not be 00; serial must not be 0000.
		Validate: ssnValid,
	},
	{
		ID:          "email-address",
		Description: "Email address (RFC-5322 lite — local@domain.tld)",
		Regex:       regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
	},
	{
		ID:          "phone-number",
		Description: "Phone number (US-style or international with country code or area-code parens)",
		// Conservative: requires either a + country code OR a 10-digit US shape with separator.
		// Leading \b is intentionally absent — `(` and `+` are non-word chars
		// so a left word-boundary before them never fires; trailing \b is
		// kept only on the digit-only branch to avoid swallowing trailing
		// digits.
		Regex: regexp.MustCompile(
			`(?:` +
				`\+\d{1,3}[\s.\-]?\d{1,4}[\s.\-]?\d{2,4}[\s.\-]?\d{2,4}(?:[\s.\-]?\d{0,4})?` + // intl
				`|\(\d{3}\)\s?\d{3}[\s.\-]\d{4}` + // (123) 456-7890
				`|\b\d{3}[.\-]\d{3}[.\-]\d{4}\b` + // 123-456-7890 / 123.456.7890
				`)`,
		),
	},
}

// luhnValid implements the standard Luhn checksum (mod-10) used by major
// credit-card networks.
func luhnValid(digits string) bool {
	n := len(digits)
	if n < 12 || n > 19 {
		return false
	}
	sum := 0
	double := false
	for i := n - 1; i >= 0; i-- {
		c := digits[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// ibanValid implements ISO 13616 MOD-97. Letters map to 10–35; the country
// code + check digits get moved to the end before the modulo is computed.
func ibanValid(s string) bool {
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	// Move first 4 chars to the end.
	rotated := s[4:] + s[:4]

	// Convert each char to its numeric form: digits stay, letters → A=10..Z=35.
	var b strings.Builder
	for _, r := range rotated {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteString(strconv.Itoa(int(r - 'A' + 10)))
		default:
			return false
		}
	}

	// Compute MOD-97 piecewise to avoid big-int (numeric form is up to ~70 digits).
	num := b.String()
	rem := 0
	for i := 0; i < len(num); i++ {
		rem = (rem*10 + int(num[i]-'0')) % 97
	}
	return rem == 1
}

func ssnValid(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 3 || len(parts[0]) != 3 || len(parts[1]) != 2 || len(parts[2]) != 4 {
		return false
	}
	if parts[0] == "000" || parts[0] == "666" || parts[0][0] == '9' {
		return false
	}
	if parts[1] == "00" {
		return false
	}
	if parts[2] == "0000" {
		return false
	}
	return true
}

func stripNonDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchPII returns the first PII pattern that fires (and survives validation)
// against text. Returns "" / false if nothing matches.
func matchPII(text string) (id string, ok bool) {
	for _, p := range piiPatterns {
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

// PIIDescriptions exposes the catalog for `pc-cli doctor` / future tooling.
func PIIDescriptions() []struct{ ID, Description string } {
	out := make([]struct{ ID, Description string }, 0, len(piiPatterns))
	for _, p := range piiPatterns {
		out = append(out, struct{ ID, Description string }{p.ID, p.Description})
	}
	return out
}
