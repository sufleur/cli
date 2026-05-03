package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

// Cache stores prompt data as JSON files on disk. Filenames embed the resolved
// version so multiple versions of the same underlying package coexist:
//
//	.sufleur/@workspace__name@1.2.3.json
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

// Dir returns the cache directory path.
func (c *Cache) Dir() string { return c.dir }

// Key derives the on-disk key (no extension) for a (ref, version) pair.
// Ref takes precedence; falls back to Name when Ref is empty.
func Key(ref, name, version string) string {
	base := ref
	if base == "" {
		base = name
	}
	base = strings.ReplaceAll(base, "/", "__")
	return base + "@" + version
}

// keyFromData derives the cache key from a populated PromptData.
func keyFromData(pd *generator.PromptData) string {
	return Key(pd.Ref, pd.Name, pd.Version)
}

// Store writes prompt data to <dir>/<key>.json. The key embeds the version.
func (c *Cache) Store(pd *generator.PromptData) error {
	if pd.Version == "" {
		return fmt.Errorf("cache: cannot store prompt %q with empty version", pd.Name)
	}
	data, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling prompt data: %w", err)
	}
	path := filepath.Join(c.dir, keyFromData(pd)+".json")
	return os.WriteFile(path, data, 0644)
}

// Load reads prompt data for the given key from the cache.
func (c *Cache) Load(key string) (*generator.PromptData, error) {
	path := filepath.Join(c.dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cached prompt %q: %w", key, err)
	}

	var pd generator.PromptData
	if err := json.Unmarshal(data, &pd); err != nil {
		return nil, fmt.Errorf("parsing cached prompt %q: %w", key, err)
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
		key := strings.TrimSuffix(entry.Name(), ".json")
		pd, err := c.Load(key)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, *pd)
	}
	return prompts, nil
}

// Remove deletes the cached data for the given key.
func (c *Cache) Remove(key string) error {
	path := filepath.Join(c.dir, key+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing cached prompt %q: %w", key, err)
	}
	return nil
}

// PruneTo deletes any *.json file in the cache directory whose key is not in
// the keep set. Used after Install to clean up cache files for prompts (or
// versions) that have been removed from the lockfile.
func (c *Cache) PruneTo(keep map[string]bool) error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("reading cache dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		if keep[key] {
			continue
		}
		if err := os.Remove(filepath.Join(c.dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("pruning cache entry %q: %w", key, err)
		}
	}
	return nil
}
