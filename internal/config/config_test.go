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
			"wtomas":    "test-key-123",
			"acme-corp": "test-key-456",
		},
		Prompts: map[string]string{
			"@wtomas/greeting":    "^1.0.0",
			"@acme-corp/farewell": "~2.1.0",
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

func TestMissingEnvVar_DeferredToAPIKeyFor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	os.Unsetenv("NONEXISTENT_VAR")
	t.Setenv("OTHER_KEY", "other-secret")

	original := SufleurConfig{
		APIKeys: map[string]string{
			"wtomas": "${NONEXISTENT_VAR}",
			"other":  "${OTHER_KEY}",
		},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A key whose env var is unset must not block loading: other workspaces
	// (or public prompts) must remain usable.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, ok := cfg.ResolvedKeys["wtomas"]; ok {
		t.Error("expected no resolved key for workspace with unset env var")
	}
	if cfg.ResolvedKeys["other"] != "other-secret" {
		t.Errorf("ResolvedKeys[other] = %q, want %q", cfg.ResolvedKeys["other"], "other-secret")
	}

	// The error surfaces only when the broken workspace's key is requested.
	_, err = cfg.APIKeyFor("wtomas")
	if err == nil {
		t.Fatal("expected error for workspace with unset env var, got nil")
	}
	if want := "NONEXISTENT_VAR is not set"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestAPIKeyFor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sufleur.yaml")

	if err := Save(path, SufleurConfig{
		APIKeys: map[string]string{"acme": "acme-key"},
		Prompts: map[string]string{},
		Output:  OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	key, err := cfg.APIKeyFor("acme")
	if err != nil || key != "acme-key" {
		t.Errorf("APIKeyFor(acme) = (%q, %v), want (%q, nil)", key, err, "acme-key")
	}

	// No api_keys entry at all means anonymous access (public prompts).
	key, err = cfg.APIKeyFor("unconfigured")
	if err != nil || key != "" {
		t.Errorf("APIKeyFor(unconfigured) = (%q, %v), want (\"\", nil)", key, err)
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

func TestParsePromptEntry_PlainConstraint(t *testing.T) {
	got, err := ParsePromptEntry("@wtomas/foo", "^1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := PromptEntry{Alias: "@wtomas/foo", Package: "@wtomas/foo", Constraint: "^1.0.0"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.IsAlias() {
		t.Error("plain constraint should not be flagged as alias")
	}
}

func TestParsePromptEntry_DraftSentinel(t *testing.T) {
	got, err := ParsePromptEntry("@wtomas/foo", "draft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Constraint != "draft" {
		t.Errorf("Constraint = %q, want \"draft\"", got.Constraint)
	}
}

func TestParsePromptEntry_AliasSpec(t *testing.T) {
	got, err := ParsePromptEntry("@wtomas/old-foo", "@wtomas/foo@^0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := PromptEntry{Alias: "@wtomas/old-foo", Package: "@wtomas/foo", Constraint: "^0.1.0"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.IsAlias() {
		t.Error("alias spec should be flagged as alias")
	}
}

func TestParsePromptEntry_AliasDraft(t *testing.T) {
	got, err := ParsePromptEntry("@wtomas/draft-foo", "@wtomas/foo@draft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Package != "@wtomas/foo" || got.Constraint != "draft" {
		t.Errorf("got %+v, want package=@wtomas/foo constraint=draft", got)
	}
}

func TestParsePromptEntry_AliasCrossWorkspace(t *testing.T) {
	got, err := ParsePromptEntry("@me/legacy", "@wtomas/foo@^0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Alias != "@me/legacy" || got.Package != "@wtomas/foo" {
		t.Errorf("alias should not have to share workspace with package; got %+v", got)
	}
}

func TestParsePromptEntry_RejectMalformed(t *testing.T) {
	cases := []struct{ key, value string }{
		{"@wtomas/foo", ""},              // empty value
		{"@wtomas/foo", "@bare"},         // no separator
		{"@wtomas/foo", "@wtomas/foo@"},  // empty constraint
		{"@wtomas/foo", "@/foo@^1.0.0"},  // empty workspace
		{"@wtomas/foo", "@nopkg@^1.0.0"}, // missing slash in package
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if _, err := ParsePromptEntry(tc.key, tc.value); err == nil {
				t.Errorf("expected error for value %q", tc.value)
			}
		})
	}
}

func TestPromptEntries_StableOrder(t *testing.T) {
	cfg := SufleurConfig{
		Prompts: map[string]string{
			"@b/two":  "^0.2.0",
			"@a/one":  "^0.1.0",
			"@a/zero": "@a/one@^0.0.5",
		},
	}
	entries, err := cfg.PromptEntries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len = %d, want 3", len(entries))
	}
	wantAliases := []string{"@a/one", "@a/zero", "@b/two"}
	for i, want := range wantAliases {
		if entries[i].Alias != want {
			t.Errorf("entries[%d].Alias = %q, want %q", i, entries[i].Alias, want)
		}
	}
	if !entries[1].IsAlias() {
		t.Error("@a/zero entry should be flagged as alias")
	}
	if entries[1].Package != "@a/one" || entries[1].Constraint != "^0.0.5" {
		t.Errorf("alias spec parse wrong: %+v", entries[1])
	}
}

func TestFormatPromptValue(t *testing.T) {
	if got := FormatPromptValue("@x/y", "@x/y", "^1.0.0"); got != "^1.0.0" {
		t.Errorf("non-alias: got %q", got)
	}
	if got := FormatPromptValue("@x/old", "@x/y", "^0.1.0"); got != "@x/y@^0.1.0" {
		t.Errorf("alias: got %q", got)
	}
}
