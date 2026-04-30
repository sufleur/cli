package cache

import (
	"path/filepath"
	"testing"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

func samplePrompt(name string) *generator.PromptData {
	return &generator.PromptData{
		Name:        name,
		Version:     "1.0.0",
		Description: "Test prompt",
		Files: []generator.PromptFile{
			{Name: "main.txt", Content: "Hello"},
		},
	}
}

func TestRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	c, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pd := samplePrompt("greeting")
	if err := c.Store(pd); err != nil {
		t.Fatalf("Store: %v", err)
	}

	loaded, err := c.Load("greeting")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != pd.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, pd.Name)
	}
	if loaded.Version != pd.Version {
		t.Errorf("Version = %q, want %q", loaded.Version, pd.Version)
	}
	if len(loaded.Files) != 1 || loaded.Files[0].Content != "Hello" {
		t.Errorf("unexpected Files: %+v", loaded.Files)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadAll(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := c.Store(samplePrompt(name)); err != nil {
			t.Fatalf("Store(%s): %v", name, err)
		}
	}

	all, err := c.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("LoadAll count = %d, want 3", len(all))
	}
}

func TestRemove(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Store(samplePrompt("removeme")); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := c.Remove("removeme"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err = c.Load("removeme")
	if err == nil {
		t.Fatal("expected error after remove, got nil")
	}
}

func TestRemove_Nonexistent(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Should not error for nonexistent file
	if err := c.Remove("nonexistent"); err != nil {
		t.Fatalf("Remove nonexistent: %v", err)
	}
}

func TestStore_WithRef(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pd := &generator.PromptData{
		Ref:         "@wtomas/my-prompt",
		Name:        "my-prompt",
		Version:     "1.0.0",
		Description: "Test prompt with ref",
		Files: []generator.PromptFile{
			{Name: "main.txt", Content: "Hello"},
		},
	}

	if err := c.Store(pd); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Should be loadable via sanitized ref key
	loaded, err := c.Load("@wtomas__my-prompt")
	if err != nil {
		t.Fatalf("Load by sanitized ref: %v", err)
	}
	if loaded.Ref != "@wtomas/my-prompt" {
		t.Errorf("Ref = %q, want %q", loaded.Ref, "@wtomas/my-prompt")
	}
	if loaded.Name != "my-prompt" {
		t.Errorf("Name = %q, want %q", loaded.Name, "my-prompt")
	}
}

func TestLoadAll_WithRef(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pd := &generator.PromptData{
		Ref:     "@acme/greeting",
		Name:    "greeting",
		Version: "2.0.0",
		Files:   []generator.PromptFile{{Name: "userPrompt", Content: "Hi"}},
	}
	if err := c.Store(pd); err != nil {
		t.Fatalf("Store: %v", err)
	}

	all, err := c.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if len(all) != 1 {
		t.Fatalf("LoadAll count = %d, want 1", len(all))
	}
	if all[0].Ref != "@acme/greeting" {
		t.Errorf("Ref = %q, want %q", all[0].Ref, "@acme/greeting")
	}
}
