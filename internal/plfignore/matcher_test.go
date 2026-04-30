package plfignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// ─── Layered matcher (baseline + ignore + allow) ─────────────────────────────

func TestLayeredBaselineCatchesCommonSecrets(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	ignore, baseline, allow := m.Counts()
	if ignore != 0 {
		t.Errorf("expected 0 ignore patterns in clean dir, got %d", ignore)
	}
	if allow != 0 {
		t.Errorf("expected 0 allow patterns in clean dir, got %d", allow)
	}
	if baseline == 0 {
		t.Fatal("expected baseline patterns to be loaded; got 0")
	}

	// Synthetic, well-formed (but invalid) tokens — exercise each prefix.
	cases := []struct {
		name   string
		input  string
		wantID string
	}{
		{
			name:   "github classic PAT",
			input:  "here is my token: ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789",
			wantID: "github-pat-classic",
		},
		{
			name:   "github fine-grained PAT",
			input:  "github_pat_" + strings.Repeat("A", 82),
			wantID: "github-pat-fine-grained",
		},
		{
			name:   "AWS access key id (AKIA)",
			input:  "credentials: AKIAIOSFODNN7EXAMPLE",
			wantID: "aws-access-key-id",
		},
		{
			name:   "anthropic api key",
			input:  "ANTHROPIC_API_KEY=sk-ant-api03-" + strings.Repeat("A", 80),
			wantID: "anthropic-api-key",
		},
		{
			name:   "stripe live secret key",
			input:  "STRIPE_KEY=sk_live_" + strings.Repeat("a", 30),
			wantID: "stripe-secret-live",
		},
		{
			name:   "slack bot token",
			input:  "xoxb-1234-5678-abcdefghijklmno",
			wantID: "slack-token",
		},
		{
			name:   "JWT",
			input:  "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMSJ9.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			wantID: "jwt",
		},
		{
			name:   "PEM private key header",
			input:  "-----BEGIN OPENSSH PRIVATE KEY-----\nMIIEvAIBADAN...",
			wantID: "private-key-pem",
		},
		{
			name:   "DB url with credentials",
			input:  "DATABASE_URL=postgres://app:hunter2@db.internal:5432/prod",
			wantID: "db-url-with-password",
		},
		{
			// Real GCP API keys are exactly AIza + 35 chars = 39 total.
			name:   "GCP API key (AIza)",
			input:  "GOOGLE_API_KEY=AIza" + strings.Repeat("x", 35),
			wantID: "gcp-api-key",
		},
		{
			name:   "generic API_KEY assignment",
			input:  "config: api_key = abcdef0123456789ABCDEF",
			wantID: "generic-secret-assignment",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := m.Match(c.input)
			if !got.Excluded {
				t.Fatalf("expected excluded=true for %q, got Result=%+v", c.input, got)
			}
			if got.Source != SourceBaseline {
				t.Errorf("expected Source=baseline, got %q", got.Source)
			}
			if got.PatternID != c.wantID {
				t.Errorf("expected PatternID=%q, got %q", c.wantID, got.PatternID)
			}
		})
	}
}

func TestLayeredCleanPromptCaptures(t *testing.T) {
	dir := t.TempDir()
	m, _ := LoadAll(dir)
	got := m.Match("refactor the auth middleware to use the new session API")
	if got.Excluded {
		t.Fatalf("expected clean prompt to be captured, got Excluded with PatternID=%q", got.PatternID)
	}
}

func TestLayeredAllowOverridesBaseline(t *testing.T) {
	dir := t.TempDir()
	// .promptcellarallow whitelists docs/-prefixed paths so a sample token
	// inside docs/auth-tokens.md isn't filtered.
	if err := os.WriteFile(filepath.Join(dir, AllowFilename), []byte(`# Whitelist documentation examples that mention secret-shaped strings
id: docs-examples
\bdocs/\S+\.md\b
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := LoadAll(dir)
	// This input contains a real-shaped GitHub PAT but also "docs/foo.md"
	// (the allow trigger), so it should pass.
	allowed := m.Match("see docs/auth-tokens.md for an example like ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789")
	if allowed.Excluded {
		t.Fatalf("expected allow to override baseline, got %+v", allowed)
	}
	// Same shape but no allow trigger should still be excluded.
	blocked := m.Match("here it is: ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789")
	if !blocked.Excluded || blocked.Source != SourceBaseline {
		t.Fatalf("expected baseline to fire when allow doesn't match, got %+v", blocked)
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
	// A prompt that triggers BOTH the team's deny rule and the allow rule
	// should still be excluded — team rules are authoritative.
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

func TestLayeredCounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, IgnoreFilename), []byte("id: a\nfoo\nid: b\nbar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, AllowFilename), []byte("baz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := LoadAll(dir)
	ignore, baseline, allow := m.Counts()
	if ignore != 2 {
		t.Errorf("ignore=%d, want 2", ignore)
	}
	if allow != 1 {
		t.Errorf("allow=%d, want 1", allow)
	}
	if baseline < 20 { // sanity-check: we expect a comprehensive baseline
		t.Errorf("baseline=%d, expected ≥ 20", baseline)
	}
}

func TestBaselineDescriptionsExposedForDoctor(t *testing.T) {
	descs := BaselineDescriptions()
	if len(descs) < 20 {
		t.Fatalf("expected ≥ 20 baseline descriptions, got %d", len(descs))
	}
	seen := map[string]bool{}
	for _, d := range descs {
		if d.ID == "" {
			t.Errorf("baseline pattern with empty ID")
		}
		if d.Description == "" {
			t.Errorf("baseline pattern %q has empty description", d.ID)
		}
		if seen[d.ID] {
			t.Errorf("duplicate baseline ID: %q", d.ID)
		}
		seen[d.ID] = true
	}
}
