package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur-lock.yaml")

	now := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)

	original := &Lockfile{
		Resolved: map[string]ResolvedPrompt{
			"greeting": {
				Version:      "1.2.3",
				IntegritySHA: "sha256-abc123",
				Constraint:   "^1.0.0",
				Status:       "PUBLISHED",
				ResolvedAt:   now,
			},
			"farewell": {
				Version:      "2.1.0",
				IntegritySHA: "sha256-def456",
				Constraint:   "~2.1.0",
				Status:       "PUBLISHED",
				ResolvedAt:   now,
			},
		},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Resolved) != 2 {
		t.Fatalf("Resolved count = %d, want 2", len(loaded.Resolved))
	}

	g := loaded.Resolved["greeting"]
	if g.Version != "1.2.3" {
		t.Errorf("greeting.Version = %q, want %q", g.Version, "1.2.3")
	}
	if g.IntegritySHA != "sha256-abc123" {
		t.Errorf("greeting.IntegritySHA = %q, want %q", g.IntegritySHA, "sha256-abc123")
	}
	if g.Constraint != "^1.0.0" {
		t.Errorf("greeting.Constraint = %q, want %q", g.Constraint, "^1.0.0")
	}
	if g.Status != "PUBLISHED" {
		t.Errorf("greeting.Status = %q, want %q", g.Status, "PUBLISHED")
	}
}

func TestTimestampPreservation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur-lock.yaml")

	ts := time.Date(2026, 1, 15, 14, 30, 45, 0, time.UTC)

	original := &Lockfile{
		Resolved: map[string]ResolvedPrompt{
			"test-prompt": {
				Version:      "1.0.0",
				IntegritySHA: "sha256-test",
				Constraint:   "^1.0.0",
				Status:       "PUBLISHED",
				ResolvedAt:   ts,
			},
		},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := loaded.Resolved["test-prompt"].ResolvedAt
	if !got.Equal(ts) {
		t.Errorf("ResolvedAt = %v, want %v", got, ts)
	}
}

func TestAliasedEntry_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur-lock.yaml")
	now := time.Now().UTC()

	original := &Lockfile{
		Resolved: map[string]ResolvedPrompt{
			"@wtomas/legacy-foo": {
				Package:      "@wtomas/foo",
				Version:      "0.1.5",
				IntegritySHA: "sha256-abc",
				Constraint:   "^0.1.0",
				Status:       "PUBLISHED",
				ResolvedAt:   now,
			},
			"@wtomas/foo": {
				// non-aliased entry — Package left empty
				Version:      "0.2.1",
				IntegritySHA: "sha256-def",
				Constraint:   "^0.2.0",
				Status:       "PUBLISHED",
				ResolvedAt:   now,
			},
		},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	yaml := string(data)
	if !strings.Contains(yaml, "package: '@wtomas/foo'") && !strings.Contains(yaml, "package: \"@wtomas/foo\"") {
		t.Errorf("aliased entry should serialize the package field; got:\n%s", yaml)
	}
	// Non-aliased entry must NOT include a `package:` line so existing
	// lockfiles stay byte-equivalent for non-aliased projects.
	fooSection := extractSection(yaml, "@wtomas/foo:", "@wtomas/legacy-foo:")
	if strings.Contains(fooSection, "package:") {
		t.Errorf("non-aliased entry should omit the package field; got section:\n%s", fooSection)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Resolved["@wtomas/legacy-foo"].Package != "@wtomas/foo" {
		t.Errorf("Package field lost on round-trip: %+v", loaded.Resolved["@wtomas/legacy-foo"])
	}
	if loaded.Resolved["@wtomas/foo"].Package != "" {
		t.Errorf("Package should round-trip empty for non-aliased entry: %+v", loaded.Resolved["@wtomas/foo"])
	}
}

func extractSection(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i:]
	j := strings.Index(rest, end)
	if j < 0 {
		return rest
	}
	return rest[:j]
}

func TestEmptyResolvedMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur-lock.yaml")

	original := NewLockfile()

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Resolved == nil {
		t.Fatal("Resolved map should not be nil")
	}
	if len(loaded.Resolved) != 0 {
		t.Errorf("Resolved count = %d, want 0", len(loaded.Resolved))
	}
}
