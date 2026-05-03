package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

func samplePrompt(name, version string) *generator.PromptData {
	return &generator.PromptData{
		Name:        name,
		Version:     version,
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

	pd := samplePrompt("greeting", "1.0.0")
	if err := c.Store(pd); err != nil {
		t.Fatalf("Store: %v", err)
	}

	loaded, err := c.Load(Key("", "greeting", "1.0.0"))
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

func TestStore_RequiresVersion(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Store(&generator.PromptData{Name: "x"}); err == nil {
		t.Fatal("expected error storing prompt with empty Version")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.Load("nonexistent@1.0.0")
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
		if err := c.Store(samplePrompt(name, "1.0.0")); err != nil {
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

func TestLoadAll_DifferentVersions(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, v := range []string{"0.1.0", "0.2.0"} {
		if err := c.Store(&generator.PromptData{
			Ref: "@acme/multi", Name: "multi", Version: v,
			Files: []generator.PromptFile{{Name: "userPrompt", Content: "v" + v, IsEntrypoint: true}},
		}); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}
	all, err := c.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("LoadAll count = %d, want 2", len(all))
	}
}

func TestRemove(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Store(samplePrompt("removeme", "1.0.0")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	key := Key("", "removeme", "1.0.0")

	if err := c.Remove(key); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := c.Load(key); err == nil {
		t.Fatal("expected error after remove, got nil")
	}
}

func TestRemove_Nonexistent(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Remove("nonexistent@1.0.0"); err != nil {
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

	loaded, err := c.Load(Key("@wtomas/my-prompt", "my-prompt", "1.0.0"))
	if err != nil {
		t.Fatalf("Load by sanitized ref+version: %v", err)
	}
	if loaded.Ref != "@wtomas/my-prompt" {
		t.Errorf("Ref = %q, want %q", loaded.Ref, "@wtomas/my-prompt")
	}
	if loaded.Name != "my-prompt" {
		t.Errorf("Name = %q, want %q", loaded.Name, "my-prompt")
	}

	// On-disk filename should be @wtomas__my-prompt@1.0.0.json
	got, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name() != "@wtomas__my-prompt@1.0.0.json" {
		t.Errorf("filename: %v", got)
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

func TestPruneTo(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, v := range []string{"0.1.0", "0.2.0", "0.3.0"} {
		if err := c.Store(&generator.PromptData{
			Ref: "@x/y", Name: "y", Version: v,
			Files: []generator.PromptFile{{Name: "userPrompt", Content: "v" + v, IsEntrypoint: true}},
		}); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	keep := map[string]bool{Key("@x/y", "y", "0.2.0"): true}
	if err := c.PruneTo(keep); err != nil {
		t.Fatalf("PruneTo: %v", err)
	}

	all, err := c.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 1 || all[0].Version != "0.2.0" {
		t.Errorf("expected only 0.2.0 to remain, got %v", all)
	}
}
