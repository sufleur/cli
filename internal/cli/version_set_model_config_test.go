package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempModelConfigFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "model-config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestParseModelConfigFile(t *testing.T) {
	t.Run("valid file applies expected config", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: anthropic\nmodel: claude-sonnet-4-5\nparameters:\n  temperature: 0.7\n")
		mc, err := parseModelConfigFile(path)
		if err != nil {
			t.Fatalf("parseModelConfigFile: %v", err)
		}
		if mc.Provider != "ANTHROPIC" {
			t.Errorf("provider = %q, want ANTHROPIC", mc.Provider)
		}
		if mc.Model != "claude-sonnet-4-5" {
			t.Errorf("model = %q", mc.Model)
		}
		if mc.Parameters["temperature"] != 0.7 {
			t.Errorf("parameters = %+v", mc.Parameters)
		}
	})

	t.Run("accepts lowercase provider token as written by version dump", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: anthropic\nmodel: claude-sonnet-4-5\nparameters: {}\n")
		mc, err := parseModelConfigFile(path)
		if err != nil {
			t.Fatalf("parseModelConfigFile: %v", err)
		}
		if mc.Provider != "ANTHROPIC" {
			t.Errorf("provider = %q, want ANTHROPIC", mc.Provider)
		}
	})

	t.Run("invalid provider token errors", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: bedrock\nmodel: claude-sonnet-4-5\n")
		if _, err := parseModelConfigFile(path); err == nil {
			t.Fatal("expected error for invalid provider token")
		}
	})

	t.Run("missing model errors", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: anthropic\n")
		if _, err := parseModelConfigFile(path); err == nil {
			t.Fatal("expected error when model is missing")
		}
	})

	t.Run("non-object parameters errors", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: anthropic\nmodel: claude-sonnet-4-5\nparameters:\n  - 1\n  - 2\n")
		if _, err := parseModelConfigFile(path); err == nil {
			t.Fatal("expected error for non-object parameters")
		}
	})

	t.Run("invalid yaml errors", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: [anthropic\n")
		if _, err := parseModelConfigFile(path); err == nil {
			t.Fatal("expected error for invalid YAML")
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		if _, err := parseModelConfigFile(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

func TestResolveModelConfigInput(t *testing.T) {
	t.Run("from-file combined with provider flags errors", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: anthropic\nmodel: claude-sonnet-4-5\n")
		if _, err := resolveModelConfigInput(path, true, true, "anthropic", "claude-sonnet-4-5", "{}"); err == nil {
			t.Fatal("expected error when --from-file is combined with flag mode")
		}
	})

	t.Run("neither mode provided errors", func(t *testing.T) {
		if _, err := resolveModelConfigInput("", false, false, "", "", "{}"); err == nil {
			t.Fatal("expected error when neither --from-file nor flags are provided")
		}
	})

	t.Run("from-file mode applies file", func(t *testing.T) {
		path := writeTempModelConfigFile(t, "provider: anthropic\nmodel: claude-sonnet-4-5\n")
		mc, err := resolveModelConfigInput(path, true, false, "", "", "{}")
		if err != nil {
			t.Fatalf("resolveModelConfigInput: %v", err)
		}
		if mc.Provider != "ANTHROPIC" || mc.Model != "claude-sonnet-4-5" {
			t.Errorf("mc = %+v", mc)
		}
	})

	t.Run("flag mode applies flags", func(t *testing.T) {
		mc, err := resolveModelConfigInput("", false, true, "anthropic", "claude-sonnet-4-5", "{}")
		if err != nil {
			t.Fatalf("resolveModelConfigInput: %v", err)
		}
		if mc.Provider != "ANTHROPIC" || mc.Model != "claude-sonnet-4-5" {
			t.Errorf("mc = %+v", mc)
		}
	})
}

func TestParseModelConfigFlags(t *testing.T) {
	t.Run("requires provider", func(t *testing.T) {
		if _, err := parseModelConfigFlags("", "claude-sonnet-4-5", "{}"); err == nil {
			t.Fatal("expected error when --provider is missing")
		}
	})

	t.Run("rejects unknown provider", func(t *testing.T) {
		if _, err := parseModelConfigFlags("bedrock", "claude-sonnet-4-5", "{}"); err == nil {
			t.Fatal("expected error for invalid --provider token")
		}
	})

	t.Run("requires model", func(t *testing.T) {
		if _, err := parseModelConfigFlags("anthropic", "", "{}"); err == nil {
			t.Fatal("expected error when --model is missing")
		}
	})

	t.Run("rejects invalid params JSON", func(t *testing.T) {
		if _, err := parseModelConfigFlags("anthropic", "claude-sonnet-4-5", "{not json"); err == nil {
			t.Fatal("expected error for invalid --params JSON")
		}
	})

	t.Run("rejects non-object params JSON", func(t *testing.T) {
		if _, err := parseModelConfigFlags("anthropic", "claude-sonnet-4-5", "[1,2,3]"); err == nil {
			t.Fatal("expected error for --params that isn't a JSON object")
		}
	})

	t.Run("defaults params to empty object", func(t *testing.T) {
		mc, err := parseModelConfigFlags("anthropic", "claude-sonnet-4-5", "{}")
		if err != nil {
			t.Fatalf("parseModelConfigFlags: %v", err)
		}
		if len(mc.Parameters) != 0 {
			t.Errorf("parameters = %+v, want empty", mc.Parameters)
		}
	})

	t.Run("uppercases provider for the wire enum and accepts mixed case input", func(t *testing.T) {
		mc, err := parseModelConfigFlags("Anthropic", "claude-sonnet-4-5", `{"temperature": 0.7}`)
		if err != nil {
			t.Fatalf("parseModelConfigFlags: %v", err)
		}
		if mc.Provider != "ANTHROPIC" {
			t.Errorf("provider = %q, want ANTHROPIC", mc.Provider)
		}
		if mc.Model != "claude-sonnet-4-5" {
			t.Errorf("model = %q", mc.Model)
		}
		if mc.Parameters["temperature"] != 0.7 {
			t.Errorf("parameters = %+v", mc.Parameters)
		}
	})

	for _, p := range modelConfigProviders {
		p := p
		t.Run("accepts provider "+p, func(t *testing.T) {
			if _, err := parseModelConfigFlags(p, "some-model", "{}"); err != nil {
				t.Errorf("parseModelConfigFlags(%q): %v", p, err)
			}
		})
	}
}
