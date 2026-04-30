package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

// Cache stores prompt data as JSON files on disk.
type Cache struct {
	dir string
}

// New creates a cache backed by the given directory, creating it if needed.
func New(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}
	return &Cache{dir: dir}, nil
}

// cacheFilename returns the filename to use for a prompt's cache file.
// Uses Ref (sanitized: / → __) when non-empty, otherwise falls back to Name.
func cacheFilename(pd *generator.PromptData) string {
	if pd.Ref != "" {
		return strings.ReplaceAll(pd.Ref, "/", "__")
	}
	return pd.Name
}

// Store writes prompt data to <dir>/<key>.json.
func (c *Cache) Store(pd *generator.PromptData) error {
	data, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling prompt data: %w", err)
	}
	path := filepath.Join(c.dir, cacheFilename(pd)+".json")
	return os.WriteFile(path, data, 0644)
}

// Load reads prompt data for the given key from the cache.
func (c *Cache) Load(name string) (*generator.PromptData, error) {
	path := filepath.Join(c.dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cached prompt %q: %w", name, err)
	}

	var pd generator.PromptData
	if err := json.Unmarshal(data, &pd); err != nil {
		return nil, fmt.Errorf("parsing cached prompt %q: %w", name, err)
	}
	return &pd, nil
}

// LoadAll reads all cached prompt data files.
func (c *Cache) LoadAll() ([]generator.PromptData, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("reading cache dir: %w", err)
	}

	var prompts []generator.PromptData
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		pd, err := c.Load(name)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, *pd)
	}
	return prompts, nil
}

// Remove deletes the cached data for the given prompt name.
func (c *Cache) Remove(name string) error {
	path := filepath.Join(c.dir, name+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing cached prompt %q: %w", name, err)
	}
	return nil
}
