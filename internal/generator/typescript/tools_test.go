package typescript

import (
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/generator"
)

func objectSchema(props map[string]interface{}, required ...string) map[string]interface{} {
	req := make([]interface{}, len(required))
	for i, r := range required {
		req[i] = r
	}
	return map[string]interface{}{"type": "object", "properties": props, "required": req}
}

func searchPin(alias, version string) generator.ToolPin {
	return generator.ToolPin{
		Alias: alias, Ref: "@vendor/web-search", Version: version, Status: "PUBLISHED",
		ModelDescription: "Searches the web.",
		InputSchema: objectSchema(map[string]interface{}{
			"query": map[string]interface{}{"type": "string"},
		}, "query"),
	}
}

func promptWithTools(ref string, pins ...generator.ToolPin) generator.PromptData {
	return generator.PromptData{
		Ref: ref, Name: ref, Version: "1.0.0", Status: "PUBLISHED",
		Files: []generator.PromptFile{{Name: "main", Content: "Go.", IsEntrypoint: true}},
		Tools: pins,
	}
}

func TestTools_EmitsContractTypes(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent", searchPin("web_search", "1.0.0")),
	})

	assertContains(t, out, "export const VendorWebSearchToolInputSchema = z.object({")
	assertContains(t, out, "export type VendorWebSearchToolInput = z.infer<typeof VendorWebSearchToolInputSchema>;")
	assertContains(t, out, "export type VendorWebSearchToolOutput = unknown;")
	assertContains(t, out, "export type VendorWebSearchTool = (")
	assertContains(t, out, "export class ToolExecutionError extends Error {}")
	assertContains(t, out, "'@app/agent': {")
	assertContains(t, out, "'web_search': VendorWebSearchTool;")
	// zod is required for tool argument validation even with no output schemas.
	assertContains(t, out, "import { z } from 'zod';")
}

// A tool's result comes from the engineer's own code, so it is typed
// statically rather than validated.
func TestTools_OutputSchemaBecomesAStaticType(t *testing.T) {
	pin := searchPin("web_search", "1.0.0")
	pin.OutputSchema = objectSchema(map[string]interface{}{
		"hits": map[string]interface{}{"type": "integer"},
	}, "hits")

	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/agent", pin)})

	assertContains(t, out, "export type VendorWebSearchToolOutput = {")
	assertContains(t, out, "hits: number;")
	assertNotContains(t, out, "VendorWebSearchToolOutputSchema")
	// The model is told what it will get back, via the description.
	assertContains(t, out, "Returns JSON matching:")
}

func TestTools_PromptWithoutToolsMapsToNever(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent", searchPin("web_search", "1.0.0")),
		promptWithTools("@app/plain"),
	})

	assertContains(t, out, "'@app/plain': never;")
}

func TestTools_SharedContractEmittedOnce(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/a", searchPin("web_search", "1.0.0")),
		promptWithTools("@app/b", searchPin("search", "1.0.0")),
	})

	if n := strings.Count(out, "export type VendorWebSearchTool = ("); n != 1 {
		t.Errorf("expected the contract type emitted once, got %d", n)
	}
	// One contract, two wire names.
	assertContains(t, out, "'web_search': VendorWebSearchTool;")
	assertContains(t, out, "'search': VendorWebSearchTool;")
}

func TestTools_TwoVersionsSuffixBoth(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/a", searchPin("web_search", "1.2.0")),
		promptWithTools("@app/b", searchPin("web_search", "2.0.0")),
	})

	assertContains(t, out, "export type VendorWebSearchToolV1_2_0 = (")
	assertContains(t, out, "export type VendorWebSearchToolV2_0_0 = (")
	assertNotContains(t, out, "export type VendorWebSearchTool = (")
}

func TestTools_DraftPinWarns(t *testing.T) {
	draft := searchPin("fetch_page", "draft")
	draft.Status = "DRAFT"

	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/agent", draft)})

	assertContains(t, out, "const _draftTools:")
	assertContains(t, out, "'@app/agent': ['fetch_page'],")
	assertContains(t, out, "pins draft tool version(s)")
}

func TestTools_NoDraftTableWhenAllPublished(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent", searchPin("web_search", "1.0.0")),
	})

	assertNotContains(t, out, "const _draftTools:")
}

func TestTools_DispatchSurface(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent", searchPin("web_search", "1.0.0")),
	})

	assertContains(t, out, "toolDefs(): ToolDef[];")
	assertContains(t, out, "dispatchTool(name: string, rawInput: unknown, tools: ToolMapping[N]): Promise<DispatchResult>;")
	assertContains(t, out, "code: 'unknown-tool',")
	assertContains(t, out, "code: 'input-validation'")
	assertContains(t, out, "code: 'execution'")
	// Anything that is not a ToolExecutionError is a bug, not a model-visible
	// failure, and must keep its stack.
	assertContains(t, out, "throw e;")
}

// Wire names may be kebab-case, which is not a valid TS identifier.
func TestTools_KebabCaseWireNamesAreQuoted(t *testing.T) {
	pin := searchPin("fetch-page", "1.0.0")
	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/agent", pin)})

	assertContains(t, out, "'fetch-page': VendorWebSearchTool;")
}

func TestTools_RejectsIdentifierCollisionWithAPrompt(t *testing.T) {
	// The prompt yields VendorWebSearchToolOutput; so does the tool.
	prompt := generator.PromptData{
		Ref: "@vendor/web-search-tool", Name: "web-search-tool", Version: "1.0.0",
		OutputSchema: objectSchema(map[string]interface{}{"a": map[string]interface{}{"type": "string"}}),
		Files:        []generator.PromptFile{{Name: "main", Content: "x", IsEntrypoint: true}},
	}
	agent := promptWithTools("@app/agent", searchPin("web_search", "1.0.0"))
	agent.Tools[0].Ref = "@vendor/web-search"

	g := &Generator{}
	err := g.Generate(t.TempDir()+"/prompts.ts", []generator.PromptData{prompt, agent})
	if err == nil {
		t.Fatal("expected a collision error")
	}
	if !strings.Contains(err.Error(), "VendorWebSearchToolOutput") {
		t.Errorf("error should name the clashing identifier: %v", err)
	}
	if !strings.Contains(err.Error(), "--alias") {
		t.Errorf("error should suggest the way out: %v", err)
	}
}

func TestTools_RejectsInvalidWireName(t *testing.T) {
	g := &Generator{}
	err := g.Generate(t.TempDir()+"/prompts.ts", []generator.PromptData{
		promptWithTools("@app/agent", searchPin("web.search", "1.0.0")),
	})
	if err == nil {
		t.Fatal("expected an error for a wire name providers reject")
	}
}

// The no-tools path must not gain any of the tool machinery.
func TestTools_AbsentWhenNoPromptPinsAnything(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/plain")})

	for _, absent := range []string{
		"ToolExecutionError", "ToolMapping", "_toolDefs", "dispatchTool",
		"Tool Contracts", "import { z } from 'zod';",
	} {
		assertNotContains(t, out, absent)
	}
}
