package plfignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Single-file backwards-compat ────────────────────────────────────────────

func TestSingleFileLoadAndMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, IgnoreFilename)
	if err := os.WriteFile(path, []byte(`# top-level comment
id: secrets
(AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|OPENAI_API_KEY)

id: credential-shapes
(ghp_[A-Za-z0-9]{36}|sk-[A-Za-z0-9]{32,})

# unnamed pattern
\bsecurity/(runbooks|incident)\b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(m.Patterns); got != 3 {
		t.Fatalf("expected 3 patterns, got %d", got)
	}

	cases := []struct {
		text   string
		wantID string
		wantOK bool
	}{
		{"please use my GITHUB_TOKEN below", "secrets", true},
		{"see security/runbooks/onboarding.md", "", true},
		{"vanilla prompt", "", false},
	}
	for _, c := range cases {
		id, ok := m.Match(c.text)
		if ok != c.wantOK || id != c.wantID {
			t.Errorf("Match(%q) = (%q, %v), want (%q, %v)", c.text, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestSingleFileMissing(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if id, ok := m.Match("anything"); ok || id != "" {
		t.Fatalf("expected no match on empty matcher, got (%q, %v)", id, ok)
	}
}

// ─── Layered matcher: counts + clean prompt ──────────────────────────────────

func TestLayeredCleanRepoLoadsGitleaksAndPII(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	ignore, gleaks, pii, allow := m.Counts()
	if ignore != 0 {
		t.Errorf("ignore=%d, want 0 in clean dir", ignore)
	}
	if allow != 0 {
		t.Errorf("allow=%d, want 0 in clean dir", allow)
	}
	if gleaks < 200 {
		t.Errorf("gitleaks=%d, expected ≥ 200 (vendored upstream catalog)", gleaks)
	}
	if pii < 5 {
		t.Errorf("pii=%d, expected ≥ 5", pii)
	}
}

func TestLayeredCleanPromptCaptures(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)
	got := m.Match("refactor the auth middleware to use the new session API")
	if got.Excluded {
		t.Fatalf("expected clean prompt to be captured, got %+v", got)
	}
}

// ─── Gitleaks layer: catches well-known secret shapes ────────────────────────

func TestLayeredGitleaksCatchesCommonSecrets(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)

	// Synthetic but realistic-shaped fixtures.
	//
	// Constraints to satisfy gitleaks' global allowlist + per-rule entropy:
	//   - AWS access keys must be base32 alphabet only (uppercase A–Z + digits 2–7).
	//   - High-entropy threshold means we can't use long single-char repeats.
	//   - Tokens must NOT contain the literal "abcdefghijklmnopqrstuvwxyz"
	//     substring (gitleaks' global stopword for low-entropy filler).
	tokenBody := "K3xN9pQ7rT5wY1vZ4mB6jH2gF8sD0eA"
	cases := []struct {
		name      string
		input     string
		wantHit   bool
		wantIDSub string
	}{
		{
			name:      "AWS access key id",
			input:     "credentials AKIAJ2K5L7M3N6P4Q5R7 for the bucket",
			wantHit:   true,
			wantIDSub: "aws",
		},
		{
			name:      "GitHub classic PAT",
			input:     "here is my token: ghp_" + tokenBody + tokenBody[:5],
			wantHit:   true,
			wantIDSub: "github",
		},
		{
			name:      "GitHub fine-grained PAT",
			input:     "github_pat_" + strings.Repeat("K3xN9pQ7rT", 8) + "K3",
			wantHit:   true,
			wantIDSub: "github",
		},
		{
			// Anthropic format requires exactly 93 alnum chars + literal AA
			// + a delimiter. 24×3 + 21 = 93, then "AA", then trailing space.
			name:      "Anthropic API key",
			input:     "ANTHROPIC_API_KEY=sk-ant-api03-" + strings.Repeat(tokenBody[:24], 3) + tokenBody[:21] + "AA ",
			wantHit:   true,
			wantIDSub: "anthropic",
		},
		{
			name:      "Stripe live secret",
			input:     "STRIPE_KEY=sk_live_" + tokenBody,
			wantHit:   true,
			wantIDSub: "stripe",
		},
		// Slack bot token test deliberately omitted — matches gitleaks'
		// xoxb regex but also matches GitHub's push-protection scanner,
		// which blocks any commit containing one. The Slack rule's
		// behaviour is covered by gitleaks' own upstream tests.
		{
			name: "PEM private key",
			input: "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
				strings.Repeat("MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQ", 2) +
				"\n-----END OPENSSH PRIVATE KEY-----",
			wantHit:   true,
			wantIDSub: "private",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := m.Match(c.input)
			if got.Excluded != c.wantHit {
				t.Fatalf("Excluded=%v want %v; result=%+v", got.Excluded, c.wantHit, got)
			}
			if c.wantHit && c.wantIDSub != "" && !strings.Contains(strings.ToLower(got.PatternID), c.wantIDSub) {
				t.Errorf("expected pattern id to contain %q, got %q", c.wantIDSub, got.PatternID)
			}
		})
	}
}

// ─── PII layer: catches PII shapes with checksum validation where applicable ─

func TestLayeredPIICreditCardLuhn(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)

	// Visa test card 4111-1111-1111-1111 — Luhn-valid.
	got := m.Match("paid with 4111111111111111 last week")
	if !got.Excluded || got.Source != SourcePII || got.PatternID != "credit-card" {
		t.Errorf("expected PII credit-card match, got %+v", got)
	}

	// Same shape, last digit changed — Luhn invalid. Should NOT match.
	got = m.Match("order id 4111111111111112 looks similar but isn't a card")
	if got.Excluded {
		t.Errorf("expected Luhn validation to reject FP, got %+v", got)
	}
}

func TestLayeredPIIIBANChecksum(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)

	// GB82WEST12345698765432 — canonical valid IBAN (UK example, MOD-97 = 1).
	got := m.Match("send to GB82WEST12345698765432 by Friday")
	if !got.Excluded || got.PatternID != "iban" {
		t.Errorf("expected IBAN match, got %+v", got)
	}

	// Same length / format, deliberately corrupt check digits.
	got = m.Match("not actually an iban: GB99WEST12345698765432")
	if got.Excluded {
		t.Errorf("expected MOD-97 to reject corrupted IBAN, got %+v", got)
	}
}

func TestLayeredPIISSN(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)

	got := m.Match("ssn on file: 123-45-6789")
	if !got.Excluded || got.PatternID != "us-ssn" {
		t.Errorf("expected SSN match, got %+v", got)
	}

	// 000-area is reserved, regex itself rejects.
	got = m.Match("test data 000-12-3456 is not a real ssn")
	if got.Excluded {
		t.Errorf("expected regex to reject 000-area SSN, got %+v", got)
	}
}

func TestLayeredPIIEmail(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)
	got := m.Match("ping me at jane.doe+work@example.com if needed")
	if !got.Excluded || got.PatternID != "email-address" {
		t.Errorf("expected email match, got %+v", got)
	}
}

func TestLayeredPIIPhone(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)
	cases := []string{
		"call (555) 123-4567 tomorrow",
		"international: +44 20 7946 0958 works too",
		"or 555-123-4567 if local",
	}
	for _, in := range cases {
		got := m.Match(in)
		if !got.Excluded || got.PatternID != "phone-number" {
			t.Errorf("input=%q  expected phone match, got %+v", in, got)
		}
	}
}

// ─── Layer precedence ────────────────────────────────────────────────────────

func TestLayeredAllowOverridesBuiltins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AllowFilename), []byte(`# Whitelist documentation examples that mention secret-shaped strings
id: docs-examples
\bdocs/\S+\.md\b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := LoadAll(dir)

	// docs path + a real-shaped GitHub PAT → allow wins, captured.
	pat := "ghp_K3xN9pQ7rT5wY1vZ4mB6jH2gF8sD0eAK3xNz"
	got := m.Match("see docs/auth-tokens.md for an example like " + pat)
	if got.Excluded {
		t.Errorf("expected allow to override gitleaks layer, got %+v", got)
	}

	// Same prompt but no docs/ trigger → gitleaks fires.
	got = m.Match("here it is: " + pat)
	if !got.Excluded || got.Source != SourceGitleaks {
		t.Errorf("expected gitleaks to fire when allow doesn't match, got %+v", got)
	}

	// Allow also overrides PII.
	got = m.Match("test fixture in docs/payment-tests.md uses 4111111111111111")
	if got.Excluded {
		t.Errorf("expected allow to override PII layer, got %+v", got)
	}
}

func TestLayeredIgnoreBeatsAllow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, IgnoreFilename), []byte(`id: team-deny
internal-only-marker
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, AllowFilename), []byte(`id: docs-examples
\bdocs/\S+\.md\b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := LoadAll(dir)

	got := m.Match("see docs/runbook.md for the internal-only-marker procedure")
	if !got.Excluded {
		t.Fatalf("expected ignore-driven exclusion, got %+v", got)
	}
	if got.Source != SourceIgnore {
		t.Errorf("expected Source=ignore, got %q", got.Source)
	}
	if got.PatternID != "team-deny" {
		t.Errorf("expected PatternID=team-deny, got %q", got.PatternID)
	}
}

func TestPIIDescriptionsExposed(t *testing.T) {
	descs := PIIDescriptions()
	if len(descs) < 5 {
		t.Fatalf("expected ≥ 5 PII descriptions, got %d", len(descs))
	}
	seen := map[string]bool{}
	for _, d := range descs {
		if d.ID == "" || d.Description == "" {
			t.Errorf("PII pattern with empty ID or description: %+v", d)
		}
		if seen[d.ID] {
			t.Errorf("duplicate PII ID: %q", d.ID)
		}
		seen[d.ID] = true
	}
}
