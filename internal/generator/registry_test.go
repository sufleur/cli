package generator

import "testing"

type stubGenerator struct{}

func (s *stubGenerator) Generate(outFile string, prompts []PromptData) error {
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	Register("testlang", func() Generator { return &stubGenerator{} })

	g, err := Get("testlang")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if g == nil {
		t.Fatal("Get returned nil generator")
	}
}

func TestGetUnknownLanguage(t *testing.T) {
	_, err := Get("nonexistent-lang")
	if err == nil {
		t.Fatal("expected error for unknown language, got nil")
	}
}
