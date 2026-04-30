package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	original := SufleurConfig{
		APIKeys: map[string]string{
			"wtomas":   "test-key-123",
			"acme-corp": "test-key-456",
		},
		Prompts: map[string]string{
			"@wtomas/greeting":       "^1.0.0",
			"@acme-corp/farewell":    "~2.1.0",
		},
		Output: OutputConfig{
			Language: "typescript",
			File:     "./generated",
		},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Raw.APIKeys["wtomas"] != "test-key-123" {
		t.Errorf("APIKeys[wtomas] = %q, want %q", cfg.Raw.APIKeys["wtomas"], "test-key-123")
	}
	if cfg.ResolvedKeys["wtomas"] != "test-key-123" {
		t.Errorf("ResolvedKeys[wtomas] = %q, want %q", cfg.ResolvedKeys["wtomas"], "test-key-123")
	}
	if len(cfg.Raw.Prompts) != 2 {
		t.Errorf("Prompts count = %d, want 2", len(cfg.Raw.Prompts))
	}
	if cfg.Raw.Prompts["@wtomas/greeting"] != "^1.0.0" {
		t.Errorf("Prompts[@wtomas/greeting] = %q, want %q", cfg.Raw.Prompts["@wtomas/greeting"], "^1.0.0")
	}
	if cfg.Raw.Output.Language != "typescript" {
		t.Errorf("Output.Language = %q, want %q", cfg.Raw.Output.Language, "typescript")
	}
	if cfg.Raw.Output.File != "./generated" {
		t.Errorf("Output.File = %q, want %q", cfg.Raw.Output.File, "./generated")
	}
}

func TestEnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	t.Setenv("WTOMAS_API_KEY", "secret-from-env")

	original := SufleurConfig{
		APIKeys: map[string]string{
			"wtomas": "${WTOMAS_API_KEY}",
		},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Raw.APIKeys["wtomas"] != "${WTOMAS_API_KEY}" {
		t.Errorf("Raw.APIKeys[wtomas] = %q, want %q", cfg.Raw.APIKeys["wtomas"], "${WTOMAS_API_KEY}")
	}
	if cfg.ResolvedKeys["wtomas"] != "secret-from-env" {
		t.Errorf("ResolvedKeys[wtomas] = %q, want %q", cfg.ResolvedKeys["wtomas"], "secret-from-env")
	}
}

func TestMissingEnvVarError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	os.Unsetenv("NONEXISTENT_VAR")

	original := SufleurConfig{
		APIKeys: map[string]string{
			"wtomas": "${NONEXISTENT_VAR}",
		},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing env var, got nil")
	}
	if want := "NONEXISTENT_VAR is not set"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestDefaultEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	os.Unsetenv("SUFLEUR_ENDPOINT")

	if err := Save(path, SufleurConfig{
		APIKeys: map[string]string{"test": "test-key"},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ResolvedEndpoint != DefaultEndpoint {
		t.Errorf("ResolvedEndpoint = %q, want %q", cfg.ResolvedEndpoint, DefaultEndpoint)
	}
}

func TestEndpointEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	t.Setenv("SUFLEUR_ENDPOINT", "http://localhost:3001/graphql")

	if err := Save(path, SufleurConfig{
		APIKeys: map[string]string{"test": "test-key"},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ResolvedEndpoint != "http://localhost:3001/graphql" {
		t.Errorf("ResolvedEndpoint = %q, want %q", cfg.ResolvedEndpoint, "http://localhost:3001/graphql")
	}
}

func TestDotEnvLoading(t *testing.T) {
	dir := t.TempDir()

	// Write a .env file
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MY_TEST_KEY=from-dotenv\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// Write config referencing the var
	cfgPath := filepath.Join(dir, "sufleur.yaml")
	original := SufleurConfig{
		APIKeys: map[string]string{
			"test": "${MY_TEST_KEY}",
		},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}
	if err := Save(cfgPath, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Set the env var to simulate .env loading (actual .env loading happens in CLI root)
	t.Setenv("MY_TEST_KEY", "from-dotenv")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ResolvedKeys["test"] != "from-dotenv" {
		t.Errorf("ResolvedKeys[test] = %q, want %q", cfg.ResolvedKeys["test"], "from-dotenv")
	}
}

func TestMultipleWorkspaceKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	t.Setenv("KEY_A", "secret-a")
	t.Setenv("KEY_B", "secret-b")

	original := SufleurConfig{
		APIKeys: map[string]string{
			"workspace-a": "${KEY_A}",
			"workspace-b": "${KEY_B}",
		},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.ResolvedKeys["workspace-a"] != "secret-a" {
		t.Errorf("ResolvedKeys[workspace-a] = %q, want %q", cfg.ResolvedKeys["workspace-a"], "secret-a")
	}
	if cfg.ResolvedKeys["workspace-b"] != "secret-b" {
		t.Errorf("ResolvedKeys[workspace-b] = %q, want %q", cfg.ResolvedKeys["workspace-b"], "secret-b")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
