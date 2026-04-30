package plfignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".promptcellarignore")
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
		{"this is just text containing GITHUB_TOKEN=ghp_abc", "secrets", true},
		{"sk-AbCdEfGhIj0123456789ZyXwVuTsRqPoNm", "credential-shapes", true},
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

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if id, ok := m.Match("anything"); ok || id != "" {
		t.Fatalf("expected no match on empty matcher, got (%q, %v)", id, ok)
	}
}
