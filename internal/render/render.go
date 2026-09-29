// Package render reads a dump-style prompt directory and renders one of its
// entrypoints with Mustache. It mirrors what the codegen path emits at
// build time so an agent's `edit → render` loop produces the same output a
// downstream user would see.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cbroglie/mustache"
	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/generator"
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
	// YAMLFiles marks files dumped as <name>.yaml.mustache (YAML format).
	YAMLFiles map[string]bool
	// DecisionSpec is decision.yaml (decision prompts only), or nil.
	DecisionSpec *generator.DecisionSpec
	// Model is model-config.yaml's model, when present.
	Model string
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
	yamlFiles := make(map[string]bool)
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
		base := strings.TrimSuffix(name, ".mustache")
		if strings.HasSuffix(base, ".yaml") {
			base = strings.TrimSuffix(base, ".yaml")
			yamlFiles[base] = true
		}
		files[base] = string(raw)
	}

	pd := &PromptDir{Files: files, YAMLFiles: yamlFiles}

	if err := pd.loadDecision(dir); err != nil {
		return nil, err
	}

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

func (p *PromptDir) loadDecision(dir string) error {
	specPath := filepath.Join(dir, "decision.yaml")
	raw, err := os.ReadFile(specPath)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return fmt.Errorf("reading %s: %w", specPath, err)
	}
	tree, err := generator.ParseStructured(string(raw))
	if err != nil {
		return fmt.Errorf("parsing %s: %w", specPath, err)
	}
	specJSON, err := tree.MarshalJSON()
	if err != nil {
		return err
	}
	var spec generator.DecisionSpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return fmt.Errorf("parsing %s: %w", specPath, err)
	}
	p.DecisionSpec = &spec

	modelPath := filepath.Join(dir, "model-config.yaml")
	if raw, err := os.ReadFile(modelPath); err == nil {
		var mc struct {
			Model string `yaml:"model"`
		}
		if err := yaml.Unmarshal(raw, &mc); err != nil {
			return fmt.Errorf("parsing %s: %w", modelPath, err)
		}
		p.Model = mc.Model
	}
	return nil
}

// Render renders the entrypoint template with the given vars. All other files
// in the directory are exposed as Mustache partials. A YAML-format file
// renders to JSON (parse first, then render each string value).
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
	if !p.YAMLFiles[entrypoint] {
		return mustache.RenderPartials(prepared[entrypoint], provider, vars)
	}
	value, err := p.renderValue(entrypoint, vars, provider)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, value, "", "  "); err != nil {
		return "", err
	}
	return out.String() + "\n", nil
}

// renderValue renders one file to JSON: a string for TEXT files, the rendered
// tree for YAML files.
func (p *PromptDir) renderValue(name string, vars map[string]any, provider mustache.PartialProvider) (json.RawMessage, error) {
	content := p.substituteDirectives(p.Files[name])
	if !p.YAMLFiles[name] {
		text, err := mustache.RenderPartials(content, provider, vars)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(text); err != nil {
			return nil, err
		}
		return bytes.TrimRight(buf.Bytes(), "\n"), nil
	}
	tree, err := generator.ParseStructured(p.Files[name])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	rendered, err := generator.RenderStructured(tree, vars, func(t string) (string, error) {
		return mustache.RenderPartials(p.substituteDirectives(t), provider, vars)
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return rendered.MarshalJSON()
}

// RenderDecision builds the System-One request body ({model, state, questions})
// for a decision-prompt directory. inputsByFile holds each question/state
// file's Mustache inputs; state is the raw state for prompts without a state
// file (and must be nil otherwise).
func (p *PromptDir) RenderDecision(state any, inputsByFile map[string]map[string]any) ([]byte, error) {
	spec := p.DecisionSpec
	if spec == nil {
		return nil, fmt.Errorf("no decision.yaml in this directory — not a decision prompt")
	}
	prepared := make(map[string]string, len(p.Files))
	for name, content := range p.Files {
		prepared[name] = p.substituteDirectives(content)
	}
	provider := &mustache.StaticProvider{Partials: prepared}
	inputs := func(name string) map[string]any {
		if v := inputsByFile[name]; v != nil {
			return v
		}
		return map[string]any{}
	}

	var b bytes.Buffer
	model, _ := json.Marshal(p.Model)
	b.WriteString(`{"model":`)
	b.Write(model)
	b.WriteString(`,"state":`)
	if spec.StateFile != "" {
		if state != nil {
			return nil, fmt.Errorf("this prompt renders its state from %q — pass that file's inputs instead of a raw state", spec.StateFile)
		}
		if _, ok := p.Files[spec.StateFile]; !ok {
			return nil, fmt.Errorf("state file %q is missing from files/", spec.StateFile)
		}
		value, err := p.renderValue(spec.StateFile, inputs(spec.StateFile), provider)
		if err != nil {
			return nil, err
		}
		b.Write(value)
	} else {
		if state == nil {
			return nil, fmt.Errorf("this prompt has no state file — pass the state with --state or --state-file")
		}
		var raw bytes.Buffer
		enc := json.NewEncoder(&raw)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(state); err != nil {
			return nil, err
		}
		b.Write(bytes.TrimRight(raw.Bytes(), "\n"))
	}
	b.WriteString(`,"questions":{`)
	for i, q := range spec.Questions {
		if _, ok := p.Files[q.ID]; !ok {
			return nil, fmt.Errorf("question file %q is missing from files/", q.ID)
		}
		value, err := p.renderValue(q.ID, inputs(q.ID), provider)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		id, _ := json.Marshal(q.ID)
		typ, _ := json.Marshal(q.Type)
		b.Write(id)
		b.WriteString(`:{"type":`)
		b.Write(typ)
		b.WriteString(`,"instructions":`)
		b.Write(value)
		if len(q.Criteria) > 0 {
			b.WriteString(`,"criteria":`)
			b.Write(q.Criteria)
		}
		b.WriteByte('}')
	}
	b.WriteString("}}")

	var out bytes.Buffer
	if err := json.Indent(&out, b.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// substituteDirectives replaces `{{@outputSchema}}` with the pretty-JSON
// schema body. Mirrors internal/generator.ResolveDirectives so behavior is
// identical to what the codegen path applies.
func (p *PromptDir) substituteDirectives(content string) string {
	if p.DecisionSpec != nil {
		content = generator.ResolveFieldDirectives(content)
	}
	if !strings.Contains(content, "@outputSchema") {
		return content
	}
	var replacement string
	if p.OutputSchema != nil {
		raw, err := json.MarshalIndent(p.OutputSchema, "", "  ")
		if err == nil {
			replacement = string(raw)
		}
	}
	return generator.ReplaceOutputSchemaDirective(content, replacement)
}
