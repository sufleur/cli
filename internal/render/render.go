// Package render reads a dump-style prompt directory and renders one of its
// entrypoints with Mustache. It mirrors what the codegen path emits at
// build time so an agent's `edit → render` loop produces the same output a
// downstream user would see.
package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cbroglie/mustache"
)

// PromptDir holds templates and the optional output schema loaded from a
// dump-style directory.
type PromptDir struct {
	// Files maps the registry name (no .mustache suffix) to the raw template
	// content.
	Files map[string]string
	// OutputSchema is the parsed contents of output-schema.json, or nil when
	// the file is absent.
	OutputSchema map[string]any
}

// Load reads a dump-style directory:
//
//	<dir>/files/<name>.mustache  → Files[name]
//	<dir>/output-schema.json     → OutputSchema (optional)
func Load(dir string) (*PromptDir, error) {
	filesDir := filepath.Join(dir, "files")
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filesDir, err)
	}
	files := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".mustache") {
			continue
		}
		path := filepath.Join(filesDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		files[strings.TrimSuffix(name, ".mustache")] = string(raw)
	}

	pd := &PromptDir{Files: files}

	schemaPath := filepath.Join(dir, "output-schema.json")
	raw, err := os.ReadFile(schemaPath)
	switch {
	case err == nil:
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", schemaPath, err)
		}
		pd.OutputSchema = schema
	case os.IsNotExist(err):
		// Optional file; leave schema nil.
	default:
		return nil, fmt.Errorf("reading %s: %w", schemaPath, err)
	}

	return pd, nil
}

// Render renders the entrypoint template with the given vars. All other files
// in the directory are exposed as Mustache partials.
//
// Before rendering, `{{@outputSchema}}` is substituted with the pretty-JSON
// representation of OutputSchema (empty string when no schema is present),
// matching the codegen-time directive resolution.
func (p *PromptDir) Render(entrypoint string, vars map[string]any) (string, error) {
	if _, ok := p.Files[entrypoint]; !ok {
		return "", fmt.Errorf("entrypoint %q not found in files/", entrypoint)
	}

	prepared := make(map[string]string, len(p.Files))
	for name, content := range p.Files {
		prepared[name] = p.substituteDirectives(content)
	}

	provider := &mustache.StaticProvider{Partials: prepared}
	if vars == nil {
		vars = map[string]any{}
	}
	return mustache.RenderPartials(prepared[entrypoint], provider, vars)
}

// substituteDirectives replaces `{{@outputSchema}}` with the pretty-JSON
// schema body. Mirrors internal/generator.ResolveDirectives so behavior is
// identical to what the codegen path applies.
func (p *PromptDir) substituteDirectives(content string) string {
	if !strings.Contains(content, "{{@") {
		return content
	}
	var replacement string
	if p.OutputSchema != nil {
		raw, err := json.MarshalIndent(p.OutputSchema, "", "  ")
		if err == nil {
			replacement = string(raw)
		}
	}
	return strings.ReplaceAll(content, "{{@outputSchema}}", replacement)
}
