package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fix: `file create` with a non-.mustache file and no --name must fail
// client-side with a friendly hint, instead of sending the raw (invalid)
// filename to the backend and surfacing its "Bad Request Exception" dump. ---

func TestFileCreate_NonMustacheNoName_FriendlyError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetFileCreateFlags(t)
	_ = fileCreateCmd.Flags().Set("file", path)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // guarantee no credentials file / network call is needed

	fileCreateCmd.SetContext(context.Background())
	err := fileCreateCmd.RunE(fileCreateCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error for a non-.mustache file with no --name, got nil")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("expected the error to mention --name, got: %v", err)
	}
	if strings.Contains(err.Error(), "Bad Request") {
		t.Errorf("error should be caught client-side, not the backend's raw message: %v", err)
	}
}

func TestFileCreate_NonMustacheWithName_Allowed(t *testing.T) {
	// Only reaches the "derive name from filename" guard when --name is
	// empty; passing --name explicitly must not be rejected even though the
	// underlying file has a non-.mustache extension. It fails later (no
	// credentials configured in this test), but must NOT fail on the
	// --name guard.
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetFileCreateFlags(t)
	_ = fileCreateCmd.Flags().Set("file", path)
	_ = fileCreateCmd.Flags().Set("name", "notes")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // guarantee no credentials file exists

	fileCreateCmd.SetContext(context.Background())
	err := fileCreateCmd.RunE(fileCreateCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error (no stored credentials in test env), got nil")
	}
	if strings.Contains(err.Error(), "--name") {
		t.Errorf("passing --name explicitly should not trip the derive-name guard, got: %v", err)
	}
}

func TestFileCreate_MustacheNoName_DerivesName(t *testing.T) {
	// A .mustache file with no --name must still pass the client-side guard
	// (it only fails later, on missing credentials in this test env).
	dir := t.TempDir()
	path := filepath.Join(dir, "userPrompt.mustache")
	if err := os.WriteFile(path, []byte("hello {{name}}"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	resetFileCreateFlags(t)
	_ = fileCreateCmd.Flags().Set("file", path)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // guarantee no credentials file exists

	fileCreateCmd.SetContext(context.Background())
	err := fileCreateCmd.RunE(fileCreateCmd, []string{"@ws/prompt@1.0.0"})
	if err == nil {
		t.Fatal("expected an error (no stored credentials in test env), got nil")
	}
	if strings.Contains(err.Error(), "--name") {
		t.Errorf(".mustache files should derive a name automatically, got: %v", err)
	}
}

// resetFileCreateFlags restores fileCreateCmd's flags to their zero values so
// tests do not leak Set() calls into one another (fileCreateCmd is a package
// singleton reused across tests).
func resetFileCreateFlags(t *testing.T) {
	t.Helper()
	_ = fileCreateCmd.Flags().Set("file", "")
	_ = fileCreateCmd.Flags().Set("name", "")
	_ = fileCreateCmd.Flags().Set("entrypoint", "false")
}
