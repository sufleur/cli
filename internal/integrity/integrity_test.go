package integrity

import (
	"errors"
	"strings"
	"testing"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

func samplePromptData() *generator.PromptData {
	return &generator.PromptData{
		Name:        "greeting",
		Version:     "1.2.0",
		Description: "A greeting prompt",
		UserPromptInputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
		},
		Files: []generator.PromptFile{
			{Name: "main.txt", Content: "Hello {{name}}"},
			{Name: "system.txt", Content: "You are helpful"},
		},
	}
}

func TestCompute_Deterministic(t *testing.T) {
	pd := samplePromptData()
	h1 := Compute(pd)
	h2 := Compute(pd)
	if h1 != h2 {
		t.Errorf("non-deterministic: %q != %q", h1, h2)
	}
	if !strings.HasPrefix(h1, "sha256-") {
		t.Errorf("expected sha256- prefix, got %q", h1)
	}
}

func TestCompute_DifferentDataDifferentHash(t *testing.T) {
	pd1 := samplePromptData()
	pd2 := samplePromptData()
	pd2.Version = "2.0.0"

	if Compute(pd1) == Compute(pd2) {
		t.Error("different data should produce different hashes")
	}
}

func TestCompute_FileOrderIndependence(t *testing.T) {
	pd1 := samplePromptData()
	pd2 := &generator.PromptData{
		Name:                  pd1.Name,
		Version:               pd1.Version,
		Description:           pd1.Description,
		UserPromptInputSchema: pd1.UserPromptInputSchema,
		Files: []generator.PromptFile{
			{Name: "system.txt", Content: "You are helpful"},
			{Name: "main.txt", Content: "Hello {{name}}"},
		},
	}

	if Compute(pd1) != Compute(pd2) {
		t.Error("file order should not affect hash")
	}
}

func TestVerify_Pass(t *testing.T) {
	pd := samplePromptData()
	hash := Compute(pd)
	if err := Verify(pd, hash); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestVerify_Fail(t *testing.T) {
	pd := samplePromptData()
	err := Verify(pd, "sha256-badhash")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var ie *IntegrityError
	if !errors.As(err, &ie) {
		t.Fatalf("expected *IntegrityError, got %T", err)
	}
	if ie.PromptName != "greeting" {
		t.Errorf("PromptName = %q, want %q", ie.PromptName, "greeting")
	}
	if ie.Expected != "sha256-badhash" {
		t.Errorf("Expected = %q, want %q", ie.Expected, "sha256-badhash")
	}
}
