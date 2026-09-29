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

// RenderQuestion renders one question of a decision-prompt directory into the
// object sent under `questions.<key>` in a System-One request: {type,
// instructions, criteria}. inputs are the question's template inputs; options
// (an ordered JSON object of option key → option inputs, or nil) add options to
// an open choice. The rendering rules match the backend and generated code.
//
// A variable missing from the inputs renders as empty (as for LLM prompts); the
// returned warnings name the first one, since an empty value in a question is
// easy to miss and changes what the model answers.
func (p *PromptDir) RenderQuestion(questionID string, inputs map[string]any, options []byte) ([]byte, []string, error) {
	spec := p.DecisionSpec
	if spec == nil {
		return nil, nil, fmt.Errorf("no decision.yaml in this directory — not a decision prompt")
	}
	q, ok := spec.Question(questionID)
	if !ok {
		return nil, nil, fmt.Errorf("%q is not a question in decision.yaml (questions: %s)", questionID, strings.Join(spec.EntrypointNames(), ", "))
	}
	content, ok := p.Files[questionID]
	if !ok {
		return nil, nil, fmt.Errorf("question file %q is missing from files/", questionID)
	}
	instructions := generator.QuestionInstructions{Template: content}
	if p.YAMLFiles[questionID] {
		tree, err := generator.ParseStructured(content)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", questionID, err)
		}
		instructions = generator.QuestionInstructions{YAML: true, Tree: tree}
	}
	var added []generator.DecisionOption
	if len(bytes.TrimSpace(options)) > 0 {
		var err error
		if added, err = generator.OrderedOptions(options); err != nil {
			return nil, nil, fmt.Errorf("--options: %w", err)
		}
	}
	provider := &mustache.StaticProvider{Partials: p.Files}
	renderQ := func() (json.RawMessage, error) {
		return generator.RenderDecisionQuestion(q, instructions, inputs, added,
			func(template string, view map[string]any) (string, error) {
				return mustache.RenderPartials(template, provider, view)
			})
	}
	rendered, err := renderQ()
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	mustache.AllowMissingVariables = false
	_, strictErr := renderQ()
	mustache.AllowMissingVariables = true
	if strictErr != nil {
		warnings = append(warnings, strictErr.Error()+" — it renders as empty; pass it in --vars (or in the option's inputs)")
	}
	var out bytes.Buffer
	if err := json.Indent(&out, rendered, "", "  "); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), warnings, nil
}

// substituteDirectives replaces `{{@outputSchema}}` with the pretty-JSON
// schema body. Mirrors internal/generator.ResolveDirectives so behavior is
// identical to what the codegen path applies.
func (p *PromptDir) substituteDirectives(content string) string {
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
