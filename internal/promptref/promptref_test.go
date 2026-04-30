package promptref

import "testing"

func TestParse_Valid(t *testing.T) {
	ref, err := Parse("@wtomas/my-prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Workspace != "wtomas" {
		t.Errorf("Workspace = %q, want %q", ref.Workspace, "wtomas")
	}
	if ref.Name != "my-prompt" {
		t.Errorf("Name = %q, want %q", ref.Name, "my-prompt")
	}
	if ref.Raw != "@wtomas/my-prompt" {
		t.Errorf("Raw = %q, want %q", ref.Raw, "@wtomas/my-prompt")
	}
}

func TestParse_MissingAtPrefix(t *testing.T) {
	_, err := Parse("wtomas/my-prompt")
	if err == nil {
		t.Fatal("expected error for missing @ prefix, got nil")
	}
}

func TestParse_EmptyWorkspace(t *testing.T) {
	_, err := Parse("@/my-prompt")
	if err == nil {
		t.Fatal("expected error for empty workspace, got nil")
	}
}

func TestParse_EmptyName(t *testing.T) {
	_, err := Parse("@wtomas/")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestParse_NoSlash(t *testing.T) {
	_, err := Parse("@wtomas")
	if err == nil {
		t.Fatal("expected error for missing slash, got nil")
	}
}

func TestCacheKey(t *testing.T) {
	ref := PromptRef{Workspace: "wtomas", Name: "my-prompt", Raw: "@wtomas/my-prompt"}
	got := CacheKey(ref)
	want := "@wtomas__my-prompt"
	if got != want {
		t.Errorf("CacheKey = %q, want %q", got, want)
	}
}
