package python

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
)

// Set UPDATE_GOLDEN=1 to re-record. An env var rather than a test flag: a flag
// is rejected by sibling packages that do not declare it, so `go test ./...`
// would fail for everything else in the tree.
func updateGolden() bool { return os.Getenv("UPDATE_GOLDEN") != "" }

const (
	goldenPath      = "testdata/no_tools_snapshot.py"
	toolsGoldenPath = "testdata/tools_snapshot.py"
)

// snapshotFixturePrompts returns a fixture set that exercises every feature the
// generator supported before tool contracts existed: nested and optional and
// union input schemas, partials, draft status, output schemas, model config,
// every metadata type, a prompt with nothing set, and text that has to be
// escaped on the way into the generated file.
//
// It exists to give TestNoToolsOutputByteIdentical teeth. Adding a feature here
// is fine; removing one silently weakens the guard.
func snapshotFixturePrompts() []generator.PromptData {
	nestedInput := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string", "description": "User's name"},
					"age":  map[string]interface{}{"type": "integer"},
				},
				"required": []interface{}{"name"},
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"tone": map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"formal", "casual"},
			},
			"nickname": map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "null"},
				},
			},
		},
		"required": []interface{}{"user"},
	}

	outputSchema := map[string]interface{}{
		"type":  "object",
		"title": "Result",
		"properties": map[string]interface{}{
			"id":    map[string]interface{}{"type": "integer", "description": "Unique ID"},
			"label": map[string]interface{}{"type": "string", "minLength": float64(1)},
			"score": map[string]interface{}{"type": "number"},
			"ok":    map[string]interface{}{"type": "boolean"},
		},
		"required": []interface{}{"id", "label"},
	}

	return []generator.PromptData{
		{
			Ref:         "@acme/alpha-draft",
			Name:        "alpha-draft",
			Version:     "0.3.0",
			Description: "Draft prompt with nested, optional and union inputs",
			Status:      "DRAFT",
			Files: []generator.PromptFile{
				{
					Name:         "greeting",
					Content:      "Hi {{user.name}}!\n{{#tags}}\n- {{.}}\n{{/tags}}\n{{>signature}}",
					IsEntrypoint: true,
					InputSchema:  nestedInput,
					SchemaWarnings: []generator.SchemaWarning{
						{Path: "user.age", Message: "inferred from usage"},
					},
				},
				{
					Name:         "fallback",
					Content:      "No name given.",
					IsEntrypoint: true,
				},
				{Name: "signature", Content: "— The {{team}} team"},
				{Name: "footer", Content: "Sent by Sufleur."},
			},
		},
		{
			Ref:         "@acme/beta-published",
			Name:        "beta-published",
			Version:     "2.1.0",
			Description: "Published prompt with an output schema and full metadata",
			Status:      "PUBLISHED",
			Metadata: map[string]interface{}{
				"model":       map[string]interface{}{"type": "string", "value": "gpt-4o"},
				"retries":     map[string]interface{}{"type": "integer", "value": float64(3)},
				"temperature": map[string]interface{}{"type": "float", "value": float64(0.7)},
				"streaming":   map[string]interface{}{"type": "boolean", "value": true},
			},
			OutputSchema: outputSchema,
			ModelConfig: map[string]interface{}{
				"provider":   "anthropic",
				"model":      "claude-sonnet-4-5",
				"parameters": map[string]interface{}{"temperature": float64(0.2)},
			},
			Files: []generator.PromptFile{
				{
					Name:         "classify",
					Content:      "Classify this.\n\n{{@outputSchema}}",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{"text": map[string]interface{}{"type": "string"}},
						"required":   []interface{}{"text"},
					},
				},
			},
		},
		{
			Ref:     "@acme/gamma-bare",
			Name:    "gamma-bare",
			Version: "1.0.0",
			Status:  "PUBLISHED",
			Files: []generator.PromptFile{
				{Name: "plain", Content: "Nothing to see here.", IsEntrypoint: true},
			},
		},
		{
			Ref:         "@acme/delta-escapes",
			Name:        "delta-escapes",
			Version:     "1.0.1",
			Description: `Ends a comment */ and quotes "like this"`,
			Status:      "PUBLISHED",
			Files: []generator.PromptFile{
				{
					Name:         "tricky",
					Content:      "A backtick ` and a ${interpolation} and a backslash \\ walk into a bar.",
					IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"weird": map[string]interface{}{
								"type":        "string",
								"description": `closes a comment */ here`,
							},
						},
					},
				},
			},
		},
	}
}

// normaliseGeneratedAt blanks the only nondeterministic line in generated output.
var generatedAtRe = regexp.MustCompile(`(?m)^(#|//) Generated at: .*$`)

func normaliseGeneratedAt(s string) string {
	return generatedAtRe.ReplaceAllString(s, "$1 Generated at: <normalised>")
}

// TestNoToolsOutputByteIdentical locks the generated output for prompts that pin
// no tools. Tool-contract support adds large template branches; every one of
// them must be gated so this fixture set still produces exactly what it did
// before. Run `UPDATE_GOLDEN=1 go test ./internal/generator/...` to re-record after an
// intentional change — the diff is then reviewable in the PR.
func TestNoToolsOutputByteIdentical(t *testing.T) {
	actual := normaliseGeneratedAt(generateAndRead(t, snapshotFixturePrompts()))

	if updateGolden() {
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden (set UPDATE_GOLDEN=1 to create it): %v", err)
	}
	expected := normaliseGeneratedAt(string(want))
	if actual == expected {
		return
	}

	actualLines := strings.Split(actual, "\n")
	expectedLines := strings.Split(expected, "\n")
	for i := 0; i < len(actualLines) || i < len(expectedLines); i++ {
		var got, wantLine string
		if i < len(actualLines) {
			got = actualLines[i]
		}
		if i < len(expectedLines) {
			wantLine = expectedLines[i]
		}
		if got != wantLine {
			t.Fatalf("generated output changed at line %d\n  golden: %q\n  actual: %q\n\nIf this is intentional, re-record with: UPDATE_GOLDEN=1 go test ./internal/generator/...",
				i+1, wantLine, got)
		}
	}
	t.Fatalf("generated output differs from %s but no differing line was found", goldenPath)
}

// toolsFixturePrompts exercises the tool-contract emitter: two prompts sharing
// one contract under different wire names, a second version of the same tool
// (forcing the version-suffix rule), a draft pin, a tool with no output schema,
// and a prompt that pins nothing.
func toolsFixturePrompts() []generator.PromptData {
	searchInput := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query":      map[string]interface{}{"type": "string", "description": "What to search for"},
			"maxResults": map[string]interface{}{"type": "integer"},
		},
		"required": []interface{}{"query"},
	}
	searchOutput := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"results": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{"type": "string"},
						"url":   map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"title", "url"},
				},
			},
		},
		"required": []interface{}{"results"},
	}

	searchV1 := generator.ToolPin{
		Alias: "web_search", Ref: "@vendor/web-search", Version: "1.2.0", Status: "PUBLISHED",
		ModelDescription: "Searches the web.", InputSchema: searchInput, OutputSchema: searchOutput,
	}
	searchV2 := generator.ToolPin{
		Alias: "search", Ref: "@vendor/web-search", Version: "2.0.0", Status: "PUBLISHED",
		ModelDescription: "Searches the web, v2.", InputSchema: searchInput, OutputSchema: searchOutput,
	}
	fetchDraft := generator.ToolPin{
		Alias: "fetch-page", Ref: "@acme/fetch-page", Version: "draft", Status: "DRAFT",
		ModelDescription: "Fetches a page.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"url": map[string]interface{}{"type": "string"}},
			"required":   []interface{}{"url"},
		},
	}

	return []generator.PromptData{
		{
			Ref: "@app/daily-brief", Name: "daily-brief", Version: "1.0.0", Status: "PUBLISHED",
			Description: "Agentic prompt with tools",
			Files: []generator.PromptFile{
				{Name: "main", Content: "Research {{topic}}.", IsEntrypoint: true,
					InputSchema: map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{"topic": map[string]interface{}{"type": "string"}},
						"required":   []interface{}{"topic"},
					}},
			},
			// Same contract as @app/research pins, under a different wire name.
			Tools: []generator.ToolPin{searchV1, fetchDraft},
		},
		{
			Ref: "@app/plain", Name: "plain", Version: "1.0.0", Status: "PUBLISHED",
			Files: []generator.PromptFile{{Name: "main", Content: "No tools here.", IsEntrypoint: true}},
		},
		{
			Ref: "@app/research", Name: "research", Version: "2.0.0", Status: "PUBLISHED",
			OutputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"summary": map[string]interface{}{"type": "string"}},
				"required":   []interface{}{"summary"},
			},
			Files: []generator.PromptFile{
				{Name: "main", Content: "Summarise.", IsEntrypoint: true},
			},
			Tools: []generator.ToolPin{searchV2},
		},
	}
}

// TestToolsOutputSnapshot records the emitted file for a tools-bearing project,
// so the generated API is reviewable as a diff rather than only asserted at.
func TestToolsOutputSnapshot(t *testing.T) {
	actual := normaliseGeneratedAt(generateAndRead(t, toolsFixturePrompts()))

	if updateGolden() {
		if err := os.WriteFile(toolsGoldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		t.Logf("updated %s", toolsGoldenPath)
		return
	}

	want, err := os.ReadFile(toolsGoldenPath)
	if err != nil {
		t.Fatalf("reading golden (set UPDATE_GOLDEN=1 to create it): %v", err)
	}
	if normaliseGeneratedAt(string(want)) != actual {
		t.Errorf("generated tools output differs from %s; re-record with UPDATE_GOLDEN=1 go test ./internal/generator/...", toolsGoldenPath)
	}
}
