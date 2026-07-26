package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fix: `version set-output-schema` with a JSON value that isn't an object
// must give a friendly error, not leak the raw Go unmarshal internals
// ("json: cannot unmarshal array into Go value of type map[string]interface
// {}"). ---

func TestVersionSetOutputSchema_JSONArray_FriendlyError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, []byte(`["a", "b"]`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetVersionSetOutputSchemaFlags(t)
	_ = versionSetOutputSchemaCmd.Flags().Set("file", path)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	versionSetOutputSchemaCmd.SetContext(context.Background())
	err := versionSetOutputSchemaCmd.RunE(versionSetOutputSchemaCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error for a non-object schema, got nil")
	}
	if !strings.Contains(err.Error(), "must be a JSON object") {
		t.Errorf("expected a friendly 'must be a JSON object' message, got: %v", err)
	}
	if strings.Contains(err.Error(), "cannot unmarshal") {
		t.Errorf("error leaks the raw Go unmarshal message: %v", err)
	}
}

func TestVersionSetOutputSchema_JSONString_FriendlyError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, []byte(`"just a string"`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetVersionSetOutputSchemaFlags(t)
	_ = versionSetOutputSchemaCmd.Flags().Set("file", path)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	versionSetOutputSchemaCmd.SetContext(context.Background())
	err := versionSetOutputSchemaCmd.RunE(versionSetOutputSchemaCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error for a non-object schema, got nil")
	}
	if !strings.Contains(err.Error(), "must be a JSON object") {
		t.Errorf("expected a friendly 'must be a JSON object' message, got: %v", err)
	}
}

func TestVersionSetOutputSchema_MalformedJSON_StillReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, []byte(`{not valid json`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetVersionSetOutputSchemaFlags(t)
	_ = versionSetOutputSchemaCmd.Flags().Set("file", path)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	versionSetOutputSchemaCmd.SetContext(context.Background())
	err := versionSetOutputSchemaCmd.RunE(versionSetOutputSchemaCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("expected a parsing-error message for malformed JSON, got: %v", err)
	}
}

func TestVersionSetOutputSchema_ValidObject_PassesLocalValidation(t *testing.T) {
	// A well-formed JSON object must sail past the local JSON-shape guard;
	// it only fails later on missing credentials in this test env.
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, []byte(`{"type": "object"}`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetVersionSetOutputSchemaFlags(t)
	_ = versionSetOutputSchemaCmd.Flags().Set("file", path)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	versionSetOutputSchemaCmd.SetContext(context.Background())
	err := versionSetOutputSchemaCmd.RunE(versionSetOutputSchemaCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error (no stored credentials in test env), got nil")
	}
	if strings.Contains(err.Error(), "JSON object") {
		t.Errorf("a valid object should not trip the JSON-shape guard, got: %v", err)
	}
}

func resetVersionSetOutputSchemaFlags(t *testing.T) {
	t.Helper()
	_ = versionSetOutputSchemaCmd.Flags().Set("file", "")
}
