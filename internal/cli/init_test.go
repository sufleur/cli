package cli

import "testing"

func TestBuildInitConfig_WithWorkspace(t *testing.T) {
	cfg := buildInitConfig("acme", "ACME_API_KEY", "typescript", "./generated/prompts.ts")

	if got := cfg.APIKeys["acme"]; got != "${ACME_API_KEY}" {
		t.Errorf("APIKeys[acme] = %q, want %q", got, "${ACME_API_KEY}")
	}
	if cfg.Output.Language != "typescript" || cfg.Output.File != "./generated/prompts.ts" {
		t.Errorf("unexpected output config: %+v", cfg.Output)
	}
}

func TestBuildInitConfig_NoWorkspace_NoAPIKeys(t *testing.T) {
	// Skipping the workspace step must not write an api_keys entry keyed by
	// the empty string — public prompts are installable without any key.
	cfg := buildInitConfig("", "", "typescript", "./generated/prompts.ts")

	if len(cfg.APIKeys) != 0 {
		t.Errorf("expected no api_keys entries, got %v", cfg.APIKeys)
	}
	if cfg.Prompts == nil {
		t.Error("expected non-nil prompts map")
	}
}
