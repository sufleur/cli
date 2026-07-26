package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/config"
)

func loadTestConfig(t *testing.T, apiKeys map[string]string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")
	if err := config.Save(path, config.SufleurConfig{
		APIKeys: apiKeys,
		Prompts: map[string]string{},
		Output:  config.OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestClientForWorkspace_WithKey(t *testing.T) {
	cfg := loadTestConfig(t, map[string]string{"acme": "acme-key"})

	client, anonymous, err := clientForWorkspace(cfg, "acme", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
	if anonymous {
		t.Error("expected authenticated client when key is configured")
	}
}

func TestClientForWorkspace_NoKey_Anonymous(t *testing.T) {
	cfg := loadTestConfig(t, nil)

	client, anonymous, err := clientForWorkspace(cfg, "acme", false)
	if err != nil {
		t.Fatalf("expected anonymous client, got error: %v", err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
	if !anonymous {
		t.Error("expected anonymous client when no key is configured")
	}
}

func TestClientForWorkspace_UnresolvableKey_Fails(t *testing.T) {
	os.Unsetenv("SUFLEUR_TEST_UNSET_ADD_VAR")
	cfg := loadTestConfig(t, map[string]string{"acme": "${SUFLEUR_TEST_UNSET_ADD_VAR}"})

	_, _, err := clientForWorkspace(cfg, "acme", false)
	if err == nil {
		t.Fatal("expected error for unresolvable key, got nil")
	}
	if !strings.Contains(err.Error(), "SUFLEUR_TEST_UNSET_ADD_VAR is not set") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- fix 4: local constraint pre-validation ---

func TestValidateConstraint_Valid(t *testing.T) {
	for _, c := range []string{"*", "^1.0.0", "~2.0.0", "1.2.3", ">=1.0.0 <2.0.0", "draft"} {
		if err := validateConstraint(c); err != nil {
			t.Errorf("validateConstraint(%q) = %v, want nil", c, err)
		}
	}
}

func TestValidateConstraint_Invalid(t *testing.T) {
	for _, c := range []string{"not-a-constraint", "^abc", ""} {
		err := validateConstraint(c)
		if err == nil {
			t.Errorf("validateConstraint(%q) = nil, want error", c)
			continue
		}
		if !strings.Contains(err.Error(), "invalid version constraint") {
			t.Errorf("validateConstraint(%q) error = %v, want mention of 'invalid version constraint'", c, err)
		}
	}
}

func TestAdd_InvalidConstraint_NoNetworkCall(t *testing.T) {
	// An invalid constraint must be rejected before sufleur.yaml is even
	// read, let alone before any GraphQL round-trip.
	writeSufleurYAML(t, nil)

	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()
	t.Setenv("SUFLEUR_ENDPOINT", ts.URL)

	addCmd.SetContext(context.Background())
	_ = addCmd.Flags().Set("force", "false")
	_ = addCmd.Flags().Set("alias", "")

	err := addCmd.RunE(addCmd, []string{"@ws/foo", "not-a-constraint"})
	if err == nil {
		t.Fatal("expected error for invalid constraint, got nil")
	}
	if !strings.Contains(err.Error(), `invalid version constraint "not-a-constraint"`) {
		t.Errorf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected no HTTP call for a locally-invalid constraint")
	}
}

// --- fix 1: "Added ..." must not print before resolution succeeds ---

func TestAdd_ResolveFailure_DoesNotClaimSuccess(t *testing.T) {
	writeSufleurYAML(t, nil)
	before, err := os.ReadFile("sufleur.yaml")
	if err != nil {
		t.Fatalf("reading sufleur.yaml: %v", err)
	}

	reg := newFakeRegistry(t)
	// "foo" is intentionally absent from reg.versions, so FetchPromptVersion
	// resolves to a null version and the resolver fails.
	t.Setenv("SUFLEUR_ENDPOINT", reg.start())

	addCmd.SetContext(context.Background())
	_ = addCmd.Flags().Set("force", "false")
	_ = addCmd.Flags().Set("alias", "")

	var runErr error
	out := captureStdout(t, func() {
		runErr = addCmd.RunE(addCmd, []string{"@ws/foo"})
	})

	if runErr == nil {
		t.Fatal("expected resolve failure, got nil error")
	}
	if !strings.Contains(runErr.Error(), "sufleur.yaml was left unchanged") {
		t.Errorf("expected rollback message, got: %v", runErr)
	}
	if strings.Contains(out, "Added @ws/foo") {
		t.Errorf("stdout falsely claims success before resolution finished: %q", out)
	}
	if !strings.Contains(out, "Resolving @ws/foo") {
		t.Errorf("expected a pre-resolve status line, got: %q", out)
	}

	after, err := os.ReadFile("sufleur.yaml")
	if err != nil {
		t.Fatalf("reading sufleur.yaml after failure: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("sufleur.yaml was mutated despite rollback:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestAdd_ResolveSuccess_PrintsAddedAfterResolve(t *testing.T) {
	writeSufleurYAML(t, nil)

	reg := newFakeRegistry(t)
	reg.versions["foo"] = "1.0.0"
	t.Setenv("SUFLEUR_ENDPOINT", reg.start())

	addCmd.SetContext(context.Background())
	_ = addCmd.Flags().Set("force", "false")
	_ = addCmd.Flags().Set("alias", "")

	var runErr error
	out := captureStdout(t, func() {
		runErr = addCmd.RunE(addCmd, []string{"@ws/foo"})
	})

	if runErr != nil {
		t.Fatalf("expected success, got: %v", runErr)
	}
	if !strings.Contains(out, "Added @ws/foo (*) to sufleur.yaml") {
		t.Errorf("expected an 'Added ...' confirmation, got: %q", out)
	}

	addedIdx := strings.Index(out, "Added @ws/foo")
	resolvedIdx := strings.Index(out, "Resolved 1 prompt")
	if addedIdx == -1 || resolvedIdx == -1 || addedIdx < resolvedIdx {
		t.Errorf("expected 'Added ...' to print after resolution output, got: %q", out)
	}
}

// --- fix 3: collection add draft-only member messaging ---

func TestRunCollectionAdd_MemberHasNoPublishedVersion_FriendlyMessage(t *testing.T) {
	writeSufleurYAML(t, nil)
	before, err := os.ReadFile("sufleur.yaml")
	if err != nil {
		t.Fatalf("reading sufleur.yaml: %v", err)
	}

	reg := newFakeRegistry(t)
	reg.collections["mycollection"] = []string{"member-a", "member-b"}
	reg.versions["member-a"] = "1.0.0"
	// member-b intentionally has no version entry: it exists (it came back
	// from ListCollectionPrompts) but has no published version.
	t.Setenv("SUFLEUR_ENDPOINT", reg.start())

	addCmd.SetContext(context.Background())
	_ = addCmd.Flags().Set("force", "false")
	_ = addCmd.Flags().Set("alias", "")

	err = addCmd.RunE(addCmd, []string{"@ws/+mycollection"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	want := `@ws/member-b has no published version (draft-only) — publish it or remove it from the collection`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want to contain %q", err, want)
	}
	if strings.Contains(err.Error(), `no version of "member-b" matches constraint "*"`) {
		t.Errorf("error still leaks the raw resolver wording: %v", err)
	}

	after, err := os.ReadFile("sufleur.yaml")
	if err != nil {
		t.Fatalf("reading sufleur.yaml after failure: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("sufleur.yaml was mutated despite rollback:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRunCollectionAdd_AllMembersPublished_Succeeds(t *testing.T) {
	writeSufleurYAML(t, nil)

	reg := newFakeRegistry(t)
	reg.collections["mycollection"] = []string{"member-a", "member-b"}
	reg.versions["member-a"] = "1.0.0"
	reg.versions["member-b"] = "2.0.0"
	t.Setenv("SUFLEUR_ENDPOINT", reg.start())

	addCmd.SetContext(context.Background())
	_ = addCmd.Flags().Set("force", "false")
	_ = addCmd.Flags().Set("alias", "")

	if err := addCmd.RunE(addCmd, []string{"@ws/+mycollection"}); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	cfg, err := config.Load("sufleur.yaml")
	if err != nil {
		t.Fatalf("loading sufleur.yaml: %v", err)
	}
	if _, ok := cfg.Raw.Prompts["@ws/member-a"]; !ok {
		t.Error("expected @ws/member-a in sufleur.yaml")
	}
	if _, ok := cfg.Raw.Prompts["@ws/member-b"]; !ok {
		t.Error("expected @ws/member-b in sufleur.yaml")
	}
}
