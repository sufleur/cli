package cli

import (
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
