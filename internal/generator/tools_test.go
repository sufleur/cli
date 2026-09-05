package generator

import (
	"encoding/json"
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
