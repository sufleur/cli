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
	if ref.Version != "" {
		t.Errorf("Version = %q, want empty", ref.Version)
	}
	if ref.Raw != "@wtomas/my-prompt" {
		t.Errorf("Raw = %q, want %q", ref.Raw, "@wtomas/my-prompt")
	}
}

func TestParse_Collection(t *testing.T) {
	ref, err := Parse("@acme/+onboarding")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ref.IsCollection {
		t.Errorf("IsCollection = false, want true")
	}
	if ref.Workspace != "acme" {
		t.Errorf("Workspace = %q, want %q", ref.Workspace, "acme")
	}
	if ref.Name != "onboarding" {
		t.Errorf("Name = %q, want %q (the + must be stripped)", ref.Name, "onboarding")
	}
	if ref.Raw != "@acme/+onboarding" {
		t.Errorf("Raw = %q, want original with +", ref.Raw)
	}
}

func TestParse_PromptIsNotCollection(t *testing.T) {
	ref, err := Parse("@acme/onboarding")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.IsCollection {
		t.Errorf("IsCollection = true for a plain prompt, want false")
	}
}

func TestParse_CollectionEmptyName(t *testing.T) {
	_, err := Parse("@acme/+")
	if err == nil {
		t.Fatal("expected error for empty collection name, got nil")
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

func TestParse_RejectsVersionSuffix(t *testing.T) {
	_, err := Parse("@wtomas/my-prompt@1.2.3")
	if err == nil {
		t.Fatal("expected error for unexpected version suffix, got nil")
	}
}

func TestParseRef_NoVersion(t *testing.T) {
	ref, err := ParseRef("@wtomas/my-prompt")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if ref.Version != "" {
		t.Errorf("Version = %q, want empty", ref.Version)
	}
}

func TestParseRef_SemverVersion(t *testing.T) {
	ref, err := ParseRef("@wtomas/my-prompt@1.2.3")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if ref.Workspace != "wtomas" || ref.Name != "my-prompt" {
		t.Errorf("got %+v", ref)
	}
	if ref.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", ref.Version, "1.2.3")
	}
	if ref.Raw != "@wtomas/my-prompt@1.2.3" {
		t.Errorf("Raw = %q", ref.Raw)
	}
}

func TestParseRef_DraftVersion(t *testing.T) {
	ref, err := ParseRef("@acme/welcome@draft")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if ref.Version != "draft" {
		t.Errorf("Version = %q, want draft", ref.Version)
	}
}

func TestParseRef_EmptyVersion(t *testing.T) {
	_, err := ParseRef("@wtomas/my-prompt@")
	if err == nil {
		t.Fatal("expected error for empty version, got nil")
	}
}

func TestParseRef_MalformedInputs(t *testing.T) {
	cases := []string{
		"wtomas/my-prompt",   // missing @
		"@/my-prompt@1.0.0",  // empty workspace
		"@wtomas/@1.0.0",     // empty name
		"@wtomas",            // missing slash
		"@wtomas/my-prompt@", // empty version
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := ParseRef(c); err == nil {
				t.Errorf("expected error for %q", c)
			}
		})
	}
}
