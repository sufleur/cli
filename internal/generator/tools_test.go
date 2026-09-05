package generator

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func TestWireDescription_WithoutOutputSchema(t *testing.T) {
	got := WireDescription(ToolPin{ModelDescription: "Searches the web."})
	if got != "Searches the web." {
		t.Errorf("expected the description verbatim, got %q", got)
	}
}

func TestWireDescription_AppendsOutputSchema(t *testing.T) {
	got := WireDescription(ToolPin{
		ModelDescription: "Searches the web.",
		OutputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"hits": map[string]interface{}{"type": "integer"}},
		},
	})

	if !strings.HasPrefix(got, "Searches the web.\n\nReturns JSON matching: ") {
		t.Fatalf("expected the schema appended after the description, got %q", got)
	}
	// The suffix has to be parseable JSON: the model reads it as a schema.
	suffix := strings.TrimPrefix(got, "Searches the web.\n\nReturns JSON matching: ")
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(suffix), &parsed); err != nil {
		t.Fatalf("appended schema is not valid JSON: %v (%q)", err, suffix)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema lost its shape: %v", parsed)
	}
}

func TestWireDescription_EmptyDescriptionStillCarriesSchema(t *testing.T) {
	got := WireDescription(ToolPin{OutputSchema: map[string]interface{}{"type": "object"}})
	if !strings.Contains(got, "Returns JSON matching:") {
		t.Errorf("expected the schema even with no description, got %q", got)
	}
}

func TestAliasRe(t *testing.T) {
	cases := []struct {
		alias string
		valid bool
	}{
		{"web_search", true},
		{"web-search", true},
		{"WebSearch2", true},
		{"a", true},
		{strings.Repeat("a", 64), true},
		{"", false},
		{strings.Repeat("a", 65), false},
		{"web.search", false}, // dots are not in the provider-wide rule
		{"web search", false},
		{"web/search", false},
		{"@acme/web-search", false}, // a ref is not a wire name
	}
	for _, c := range cases {
		if got := AliasRe.MatchString(c.alias); got != c.valid {
			t.Errorf("AliasRe.MatchString(%q) = %v, want %v", c.alias, got, c.valid)
		}
	}
}

// The cache stores PromptData as JSON. A prompt that pins no tools must
// serialise to exactly the bytes it did before ToolPin existed, or every
// existing user gets a forced refetch and a --frozen CI failure on upgrade.
func TestPromptDataWithoutTools_MarshalsWithoutToolsKey(t *testing.T) {
	pd := PromptData{
		Ref:     "@acme/plain",
		Name:    "plain",
		Version: "1.0.0",
		Status:  "PUBLISHED",
		Files:   []PromptFile{{Name: "main", Content: "hi", IsEntrypoint: true}},
	}

	data, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "tools") {
		t.Errorf("a tool-free prompt must not emit a tools key, got %s", data)
	}

	// An explicitly empty slice must behave like a nil one — the fetcher relies
	// on this when a version comes back with an empty tools array.
	pd.Tools = []ToolPin{}
	empty, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(empty), "tools") {
		t.Errorf("an empty tool list must not emit a tools key, got %s", empty)
	}
}

func TestPromptDataWithTools_RoundTrips(t *testing.T) {
	pd := PromptData{
		Ref:  "@acme/agent",
		Name: "agent",
		Tools: []ToolPin{{
			Alias:            "web_search",
			Ref:              "@acme/web-search",
			Version:          "1.2.0",
			Status:           "PUBLISHED",
			ModelDescription: "Searches.",
			InputSchema:      map[string]interface{}{"type": "object"},
		}},
	}

	data, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back PromptData
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Tools) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(back.Tools))
	}
	if back.Tools[0].Alias != "web_search" || back.Tools[0].Ref != "@acme/web-search" {
		t.Errorf("pin did not round-trip: %+v", back.Tools[0])
	}
}

func pin(alias, ref, version string) ToolPin {
	return ToolPin{
		Alias: alias, Ref: ref, Version: version, Status: "PUBLISHED",
		ModelDescription: "Does a thing.",
		InputSchema:      map[string]interface{}{"type": "object"},
	}
}

func TestPlanTools_NoTools(t *testing.T) {
	plan, err := PlanTools([]PromptData{{Ref: "@acme/plain"}})
	if err != nil {
		t.Fatalf("PlanTools: %v", err)
	}
	if !plan.Empty() {
		t.Errorf("expected an empty plan, got %d keys", len(plan.Keys))
	}
}

func TestPlanTools_SingleVersionGetsBareName(t *testing.T) {
	plan, err := PlanTools([]PromptData{
		{Ref: "@acme/agent", Tools: []ToolPin{pin("web_search", "@vendor/web-search", "1.2.0")}},
	})
	if err != nil {
		t.Fatalf("PlanTools: %v", err)
	}
	if got := plan.BaseNames[ToolKey(pin("web_search", "@vendor/web-search", "1.2.0"))]; got != "VendorWebSearchTool" {
		t.Errorf("expected VendorWebSearchTool, got %q", got)
	}
	if len(plan.Renamed) != 0 {
		t.Errorf("expected no renames, got %v", plan.Renamed)
	}
}

// The same contract pinned by two prompts, under different wire names, is one
// emitted type — the alias belongs to the pin, not the contract.
func TestPlanTools_SharedContractEmitsOnce(t *testing.T) {
	plan, err := PlanTools([]PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{pin("web_search", "@vendor/web-search", "1.2.0")}},
		{Ref: "@acme/b", Tools: []ToolPin{pin("search", "@vendor/web-search", "1.2.0")}},
	})
	if err != nil {
		t.Fatalf("PlanTools: %v", err)
	}
	if len(plan.Keys) != 1 {
		t.Fatalf("expected 1 distinct contract, got %d: %v", len(plan.Keys), plan.Keys)
	}
}

func TestPlanTools_MultipleVersionsSuffixEveryMember(t *testing.T) {
	plan, err := PlanTools([]PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{pin("web_search", "@vendor/web-search", "1.2.0")}},
		{Ref: "@acme/b", Tools: []ToolPin{pin("web_search", "@vendor/web-search", "2.0.0")}},
	})
	if err != nil {
		t.Fatalf("PlanTools: %v", err)
	}

	names := []string{}
	for _, k := range plan.Keys {
		names = append(names, plan.BaseNames[k])
	}
	sort.Strings(names)
	want := []string{"VendorWebSearchToolV1_2_0", "VendorWebSearchToolV2_0_0"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("expected every version suffixed, got %v", names)
	}
	// Never a bare/suffixed mix: whose name stays bare would otherwise depend on
	// what else happened to be installed.
	for _, n := range names {
		if n == "VendorWebSearchTool" {
			t.Error("one version kept the bare name while another was suffixed")
		}
	}
	if len(plan.Renamed) != 1 || plan.Renamed[0] != "@vendor/web-search" {
		t.Errorf("expected the ref reported as renamed, got %v", plan.Renamed)
	}
}

func TestPlanTools_DraftSuffix(t *testing.T) {
	draft := pin("web_search", "@vendor/web-search", "draft")
	draft.Status = "DRAFT"
	plan, err := PlanTools([]PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{pin("web_search", "@vendor/web-search", "1.0.0")}},
		{Ref: "@acme/b", Tools: []ToolPin{draft}},
	})
	if err != nil {
		t.Fatalf("PlanTools: %v", err)
	}
	if got := plan.BaseNames[ToolKey(draft)]; got != "VendorWebSearchToolDraft" {
		t.Errorf("expected VendorWebSearchToolDraft, got %q", got)
	}
}

func TestPlanTools_EmitOrderIsStable(t *testing.T) {
	prompts := []PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{
			pin("zeta", "@vendor/zeta", "1.0.0"),
			pin("alpha", "@vendor/alpha", "2.0.0"),
		}},
		{Ref: "@acme/b", Tools: []ToolPin{pin("mid", "@vendor/alpha", "1.0.0")}},
	}
	plan, err := PlanTools(prompts)
	if err != nil {
		t.Fatalf("PlanTools: %v", err)
	}
	// Sorted by ref, then version.
	want := []string{"@vendor/alpha@1.0.0", "@vendor/alpha@2.0.0", "@vendor/zeta@1.0.0"}
	if strings.Join(plan.Keys, ",") != strings.Join(want, ",") {
		t.Errorf("emit order is not stable: got %v want %v", plan.Keys, want)
	}
}

func TestPlanTools_RejectsInvalidAlias(t *testing.T) {
	_, err := PlanTools([]PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{pin("web.search", "@vendor/web-search", "1.0.0")}},
	})
	if err == nil {
		t.Fatal("expected an error for a wire name providers reject")
	}
	if !strings.Contains(err.Error(), "web.search") || !strings.Contains(err.Error(), "@acme/a") {
		t.Errorf("error should name the wire name and the prompt: %v", err)
	}
}

func TestPlanTools_RejectsDuplicateAliasWithinAPrompt(t *testing.T) {
	_, err := PlanTools([]PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{
			pin("search", "@vendor/web-search", "1.0.0"),
			pin("search", "@vendor/other", "1.0.0"),
		}},
	})
	if err == nil {
		t.Fatal("expected an error for two tools sharing a wire name")
	}
	if !strings.Contains(err.Error(), "search") {
		t.Errorf("error should name the clashing wire name: %v", err)
	}
}

// The same wire name in two different prompts is fine — they are separate
// tool sets as far as the model is concerned.
func TestPlanTools_AllowsSameAliasAcrossPrompts(t *testing.T) {
	_, err := PlanTools([]PromptData{
		{Ref: "@acme/a", Tools: []ToolPin{pin("search", "@vendor/web-search", "1.0.0")}},
		{Ref: "@acme/b", Tools: []ToolPin{pin("search", "@vendor/other", "1.0.0")}},
	})
	if err != nil {
		t.Errorf("expected the same wire name in different prompts to be allowed: %v", err)
	}
}

func TestDraftToolAliases(t *testing.T) {
	draft := pin("fetch_page", "@acme/fetch-page", "draft")
	draft.Status = "DRAFT"
	pd := PromptData{Tools: []ToolPin{pin("web_search", "@vendor/web-search", "1.0.0"), draft}}

	got := DraftToolAliases(pd)
	if len(got) != 1 || got[0] != "fetch_page" {
		t.Errorf("expected only the draft pin, got %v", got)
	}
	if DraftToolAliases(PromptData{}) != nil {
		t.Error("expected nil for a prompt with no pins")
	}
}

func TestToPascalCase(t *testing.T) {
	cases := map[string]string{
		"@acme/web-search": "AcmeWebSearch",
		"web-search":       "WebSearch",
		"snake_case_name":  "SnakeCaseName",
		"@a/b":             "AB",
		"already":          "Already",
		"":                 "",
		"@acme/tool.name":  "AcmeTool.name",
	}
	for in, want := range cases {
		if got := ToPascalCase(in); got != want {
			t.Errorf("ToPascalCase(%q) = %q, want %q", in, got, want)
		}
	}
}
