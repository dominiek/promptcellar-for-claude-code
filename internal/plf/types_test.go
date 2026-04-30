package plf

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestRecordMarshalKeyOrder asserts top-level keys are emitted in the order
// declared in Record (which matches the PLF spec ordering).
func TestRecordMarshalKeyOrder(t *testing.T) {
	rec := &Record{
		Version:   Version,
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		SessionID: "8f14e45f-ceea-467a-9575-d68a64236d57",
		Timestamp: "2026-04-29T09:14:22.001Z",
		Author:    Author{Email: "j@example.com", Name: "J"},
		Tool:      Tool{Name: "claude-code", Version: "2.4.0"},
		Model:     Model{Provider: "anthropic", Name: "claude-opus-4-7"},
		Prompt:    "test",
	}
	buf, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`"version"`, `"id"`, `"session_id"`, `"timestamp"`,
		`"author"`, `"tool"`, `"model"`, `"prompt"`,
	}
	prev := -1
	for _, k := range want {
		idx := bytes.Index(buf, []byte(k))
		if idx < 0 {
			t.Fatalf("key %s missing in %s", k, buf)
		}
		if idx < prev {
			t.Fatalf("key %s appears out of order in %s", k, buf)
		}
		prev = idx
	}
}

func TestRecordOmitsOptionalsWhenNil(t *testing.T) {
	rec := &Record{
		Version:   Version,
		ID:        NewID(),
		SessionID: "s",
		Timestamp: "2026-04-29T00:00:00.000Z",
		Author:    Author{Email: "a@b", Name: "n"},
		Tool:      Tool{Name: "claude-code", Version: "2.0"},
		Model:     Model{Provider: "anthropic", Name: "claude-opus-4-7"},
		Prompt:    "p",
	}
	buf, _ := json.Marshal(rec)
	for _, key := range []string{`"git"`, `"parent"`, `"outcome"`} {
		if bytes.Contains(buf, []byte(key)) {
			t.Fatalf("expected %s to be omitted, got %s", key, buf)
		}
	}
}

func TestNewIDIsUUIDv4(t *testing.T) {
	id := NewID()
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 dashed parts, got %v", parts)
	}
	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("bad UUID layout: %s", id)
	}
	if parts[2][0] != '4' {
		t.Fatalf("expected version 4 nibble, got %s", id)
	}
	if !strings.ContainsAny(parts[3][:1], "89ab") {
		t.Fatalf("expected variant 10xx, got %s", id)
	}
}
