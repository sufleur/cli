package cli

import "testing"

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
