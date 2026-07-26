package cli

import (
	"testing"

	"github.com/sufleur/cli/internal/userapi"
)

func TestFormatModelConfigValue_ConfigSet(t *testing.T) {
	mc := &userapi.ModelConfig{
		Provider:   "ANTHROPIC",
		Model:      "claude-opus-4-1",
		Parameters: map[string]any{"temperature": 0.7, "max_tokens": 2000},
	}

	got := formatModelConfigValue(mc)
	want := `anthropic claude-opus-4-1 {"max_tokens":2000,"temperature":0.7}`
	if got != want {
		t.Errorf("formatModelConfigValue = %q, want %q", got, want)
	}
}

func TestFormatModelConfigValue_ParamsNil(t *testing.T) {
	mc := &userapi.ModelConfig{
		Provider:   "ANTHROPIC",
		Model:      "claude-opus-4-1",
		Parameters: nil,
	}

	got := formatModelConfigValue(mc)
	want := "anthropic claude-opus-4-1 {}"
	if got != want {
		t.Errorf("formatModelConfigValue = %q, want %q (nil Parameters must render as {} not null)", got, want)
	}
}

func TestFormatModelConfigValue_ConfigNil(t *testing.T) {
	got := formatModelConfigValue(nil)
	if got != "(none)" {
		t.Errorf("formatModelConfigValue(nil) = %q, want %q", got, "(none)")
	}
}
