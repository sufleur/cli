package python

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

	// Arguments come from the model, so they are validated at runtime.
	assertContains(t, out, "class VendorWebSearchToolInput(BaseModel):")
	assertContains(t, out, "class VendorWebSearchTool(Protocol):")
	assertContains(t, out, "def __call__(self, input: VendorWebSearchToolInput) -> Any: ...")
	assertContains(t, out, "class ToolExecutionError(Exception):")
	assertContains(t, out, "Protocol")
	// pydantic and json are needed even with no output schema anywhere.
	assertContains(t, out, "from pydantic import BaseModel, ValidationError")
	assertContains(t, out, "import json")
}

// A tool's result comes from the engineer's own code, so it is a TypedDict
// rather than a validating model.
func TestTools_OutputSchemaBecomesATypedDict(t *testing.T) {
	pin := searchPin("web_search", "1.0.0")
	pin.OutputSchema = objectSchema(map[string]interface{}{
		"hits": map[string]interface{}{"type": "integer"},
	}, "hits")

	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/agent", pin)})

	assertContains(t, out, "class VendorWebSearchToolOutput(TypedDict):")
	assertContains(t, out, "hits: int")
	assertContains(t, out, "-> VendorWebSearchToolOutput: ...")
	assertContains(t, out, "Returns JSON matching:")
}

func TestTools_BindingsUseFunctionalTypedDict(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent", searchPin("fetch-page", "1.0.0")),
	})

	// Kebab-case wire names are not valid identifiers, so class syntax is out.
	assertContains(t, out, `AppAgentTools = TypedDict("AppAgentTools", {`)
	assertContains(t, out, `"fetch-page": VendorWebSearchTool,`)
}

func TestTools_SharedContractEmittedOnce(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/a", searchPin("web_search", "1.0.0")),
		promptWithTools("@app/b", searchPin("search", "1.0.0")),
	})

	if n := strings.Count(out, "class VendorWebSearchTool(Protocol):"); n != 1 {
		t.Errorf("expected the Protocol emitted once, got %d", n)
	}
}

func TestTools_TwoVersionsSuffixBoth(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/a", searchPin("web_search", "1.2.0")),
		promptWithTools("@app/b", searchPin("web_search", "2.0.0")),
	})

	assertContains(t, out, "class VendorWebSearchToolV1_2_0(Protocol):")
	assertContains(t, out, "class VendorWebSearchToolV2_0_0(Protocol):")
	assertNotContains(t, out, "class VendorWebSearchTool(Protocol):")
}

// Dispatch is branched per tool rather than generic: a Protocol has no runtime
// value and TypedDict lookups with a non-literal key are statically `object`,
// so a generic dispatcher could not type-check the call.
func TestTools_DispatchIsBranchedPerTool(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent",
			searchPin("web_search", "1.0.0"),
			generator.ToolPin{
				Alias: "fetch-page", Ref: "@acme/fetch-page", Version: "1.0.0", Status: "PUBLISHED",
				ModelDescription: "Fetches.", InputSchema: objectSchema(map[string]interface{}{}),
			}),
	})

	assertContains(t, out, `if name == "web_search":`)
	assertContains(t, out, `if name == "fetch-page":`)
	// The kebab-case wire name cannot be a local variable name.
	assertContains(t, out, "fetch_page_input = AcmeFetchPageToolInput.model_validate(raw_input)")
	assertContains(t, out, `"code": "input-validation"`)
	assertContains(t, out, `"code": "execution"`)
	assertContains(t, out, `"code": "unknown-tool"`)
}

// get_prompt returns the dynamic result object, so the runtime lookup table is
// what actually validates — not the typed per-prompt classes.
func TestTools_RuntimeValidationTableIsEmitted(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{
		promptWithTools("@app/agent", searchPin("web_search", "1.0.0")),
	})

	assertContains(t, out, "_tool_input_models: dict[str, dict[str, type[BaseModel]]] = {")
	assertContains(t, out, `"web_search": VendorWebSearchToolInput,`)
	assertContains(t, out, "model = _tool_input_models.get(prompt_name, {}).get(name)")
	assertContains(t, out, "validated = model.model_validate(raw_input)")
}

func TestTools_DraftPinWarns(t *testing.T) {
	draft := searchPin("fetch_page", "draft")
	draft.Status = "DRAFT"

	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/agent", draft)})

	assertContains(t, out, "_draft_tools: dict[str, list[str]] = {")
	assertContains(t, out, `"@app/agent": ["fetch_page"],`)
	assertContains(t, out, "pins draft tool version(s)")
}

// An empty set literal is a dict; a project with no draft prompts must still
// get a set.
func TestTools_EmptyDraftPromptsIsASet(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/agent")})

	assertContains(t, out, "_draft_prompts: set[str] = set()")
	assertNotContains(t, out, "_draft_prompts: set[str] = {\n}")
}

func TestTools_RejectsIdentifierCollisionWithAPrompt(t *testing.T) {
	prompt := generator.PromptData{
		Ref: "@vendor/web-search-tool", Name: "web-search-tool", Version: "1.0.0",
		OutputSchema: objectSchema(map[string]interface{}{"a": map[string]interface{}{"type": "string"}}),
		Files:        []generator.PromptFile{{Name: "main", Content: "x", IsEntrypoint: true}},
	}
	// The tool needs an output schema for its Output TypedDict to be emitted;
	// that is the name that collides with the prompt's output model.
	pin := searchPin("web_search", "1.0.0")
	pin.Ref = "@vendor/web-search"
	pin.OutputSchema = objectSchema(map[string]interface{}{
		"a": map[string]interface{}{"type": "string"},
	}, "a")
	agent := promptWithTools("@app/agent", pin)

	g := &Generator{}
	err := g.Generate(t.TempDir()+"/prompts.py", []generator.PromptData{prompt, agent})
	if err == nil {
		t.Fatal("expected a collision error")
	}
	if !strings.Contains(err.Error(), "--alias") {
		t.Errorf("error should suggest the way out: %v", err)
	}
}

func TestTools_AbsentWhenNoPromptPinsAnything(t *testing.T) {
	out := generateAndRead(t, []generator.PromptData{promptWithTools("@app/plain")})

	for _, absent := range []string{
		"ToolExecutionError", "_tool_defs", "dispatch_tool", "Protocol", "Tool Contracts",
	} {
		assertNotContains(t, out, absent)
	}
}
