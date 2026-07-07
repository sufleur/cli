package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sufleur/cli/internal/userapi"
)

func TestWriteDump_WritesEvalYaml(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{
		Version:  "1.0.0",
		Files:    []userapi.PromptFile{{Name: "welcome", Content: "Hi"}},
		Readme:   "readme",
		Metadata: map[string]any{},
	}

	// Eval YAML without a trailing newline — writeDump must append one.
	if err := writeDump(dir, v, "description: hi\ndataset:\n  ref: null"); err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "eval.yaml"))
	if err != nil {
		t.Fatalf("eval.yaml not written: %v", err)
	}
	if string(raw) != "description: hi\ndataset:\n  ref: null\n" {
		t.Errorf("eval.yaml = %q (expected trailing newline appended)", string(raw))
	}

	// The rest of the dump must still be present alongside eval.yaml.
	if _, err := os.Stat(filepath.Join(dir, "files", "welcome.mustache")); err != nil {
		t.Errorf("welcome.mustache missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Errorf("README.md missing: %v", err)
	}
}

func TestWriteDump_EmptyEvalYamlStillWritesFile(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{Version: "1.0.0", Metadata: map[string]any{}}
	if err := writeDump(dir, v, ""); err != nil {
		t.Fatalf("writeDump: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "eval.yaml"))
	if err != nil {
		t.Fatalf("eval.yaml not written for empty input: %v", err)
	}
	if string(raw) != "\n" {
		t.Errorf("eval.yaml = %q, want a single newline", string(raw))
	}
}

func TestWriteDump_WritesModelConfigYaml(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{
		Version:  "1.0.0",
		Metadata: map[string]any{},
		ModelConfig: &userapi.ModelConfig{
			Provider:   "ANTHROPIC",
			Model:      "claude-sonnet-4-5",
			Parameters: map[string]any{"temperature": 0.7},
		},
	}
	if err := writeDump(dir, v, ""); err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "model-config.yaml"))
	if err != nil {
		t.Fatalf("model-config.yaml not written: %v", err)
	}
	want := "provider: anthropic\nmodel: claude-sonnet-4-5\nparameters:\n    temperature: 0.7\n"
	if string(raw) != want {
		t.Errorf("model-config.yaml = %q, want %q", string(raw), want)
	}
}

func TestWriteDump_NoModelConfigOmitsFile(t *testing.T) {
	dir := t.TempDir()
	v := &userapi.PromptVersion{Version: "1.0.0", Metadata: map[string]any{}, ModelConfig: nil}
	if err := writeDump(dir, v, ""); err != nil {
		t.Fatalf("writeDump: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "model-config.yaml")); !os.IsNotExist(err) {
		t.Errorf("model-config.yaml should not be written when ModelConfig is nil, stat err = %v", err)
	}
}
