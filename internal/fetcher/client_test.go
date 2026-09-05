package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func TestValidatePrompts_AllValid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_foo": map[string]any{"name": "foo"},
				"prompt_bar": map[string]any{"name": "bar"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	err := c.ValidatePrompts(context.Background(), []string{"foo", "bar"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidatePrompts_SomeMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {"prompt_foo": {"name": "foo"}, "prompt_bar": null, "prompt_baz": null},
			"errors": [
				{"message": "not found", "path": ["prompt_bar"]},
				{"message": "not found", "path": ["prompt_baz"]}
			]
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	err := c.ValidatePrompts(context.Background(), []string{"foo", "bar", "baz"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}

	sort.Strings(ve.MissingPrompts)
	if len(ve.MissingPrompts) != 2 || ve.MissingPrompts[0] != "bar" || ve.MissingPrompts[1] != "baz" {
		t.Fatalf("expected [bar baz], got %v", ve.MissingPrompts)
	}
}

func TestValidatePrompts_EmptyList(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	err := c.ValidatePrompts(context.Background(), []string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if called {
		t.Fatal("expected no HTTP call for empty list")
	}
}

func TestFetchPromptVersion_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {
				"prompt": {
					"description": "A test prompt",
					"version": {
						"version": "1.2.0",
						"status": "PUBLISHED",
						"metadata": {"model": "gpt-4"},
						"outputSchema": null,
						"files": [
							{
								"name": "main.txt",
								"content": "Hello {{name}}",
								"isEntrypoint": true,
								"inputSchema": {"type": "object", "properties": {"name": {"type": "string"}}},
								"schemaWarnings": []
							},
							{
								"name": "system.txt",
								"content": "You are helpful",
								"isEntrypoint": false,
								"inputSchema": null,
								"schemaWarnings": []
							}
						]
					}
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	pd, err := c.FetchPromptVersion(context.Background(), "test-prompt", "^1.0.0", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pd.Name != "test-prompt" {
		t.Errorf("expected name test-prompt, got %s", pd.Name)
	}
	if pd.Version != "1.2.0" {
		t.Errorf("expected version 1.2.0, got %s", pd.Version)
	}
	if pd.Description != "A test prompt" {
		t.Errorf("expected description 'A test prompt', got %s", pd.Description)
	}
	if len(pd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(pd.Files))
	}

	main := pd.Files[0]
	if main.Name != "main.txt" || main.Content != "Hello {{name}}" {
		t.Errorf("unexpected first file: %+v", main)
	}
	if !main.IsEntrypoint {
		t.Error("main.txt should be an entrypoint")
	}
	if main.InputSchema == nil {
		t.Error("expected non-nil InputSchema on entrypoint")
	}

	sys := pd.Files[1]
	if sys.Name != "system.txt" {
		t.Errorf("unexpected second file: %+v", sys)
	}
	if sys.IsEntrypoint {
		t.Error("system.txt should not be an entrypoint")
	}
	if sys.InputSchema != nil {
		t.Error("expected nil InputSchema on non-entrypoint")
	}

	if pd.OutputSchema != nil {
		t.Error("expected nil OutputSchema")
	}
}

func TestFetchPromptVersion_NoMatchingVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// version field is null when the constraint matches no published version
		resp := `{
			"data": {
				"prompt": {
					"description": "A test prompt",
					"version": null
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_, err := c.FetchPromptVersion(context.Background(), "test-prompt", "^99.0.0", nil)
	if err == nil {
		t.Fatal("expected error for null version, got nil")
	}

	var noVersion *NoPublishedVersionError
	if !errors.As(err, &noVersion) {
		t.Fatalf("expected *NoPublishedVersionError, got %T: %v", err, err)
	}
	if noVersion.PromptName != "test-prompt" || noVersion.Constraint != "^99.0.0" {
		t.Errorf("unexpected error fields: %+v", noVersion)
	}
}

func TestFetchPromptVersion_SchemaWarnings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {
				"prompt": {
					"description": "Has warnings",
					"version": {
						"version": "1.0.0",
						"status": "PUBLISHED",
						"metadata": {},
						"outputSchema": null,
						"files": [
							{
								"name": "userPrompt",
								"content": "Hi {{name}} {{>maybe}}",
								"isEntrypoint": true,
								"inputSchema": {"type": "object"},
								"schemaWarnings": [
									{"path": "$.partials.maybe", "message": "missing partial"}
								]
							}
						]
					}
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	pd, err := c.FetchPromptVersion(context.Background(), "warned", "1.0.0", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(pd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(pd.Files))
	}
	warns := pd.Files[0].SchemaWarnings
	if len(warns) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warns))
	}
	if warns[0].Path != "$.partials.maybe" || warns[0].Message != "missing partial" {
		t.Errorf("unexpected warning: %+v", warns[0])
	}
}

func TestFetchPromptVersion_WithOutputSchema(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {
				"prompt": {
					"description": "Structured output prompt",
					"version": {
						"version": "2.0.0",
						"status": "PUBLISHED",
						"metadata": {"model": "gpt-4o"},
						"outputSchema": {
							"type": "object",
							"properties": {
								"sentiment": {"type": "string", "enum": ["positive", "neutral", "negative"]},
								"confidence": {"type": "number"}
							},
							"required": ["sentiment", "confidence"]
						},
						"files": [
							{
								"name": "userPrompt",
								"content": "Analyze: {{text}}",
								"isEntrypoint": true,
								"inputSchema": null,
								"schemaWarnings": []
							}
						]
					}
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	pd, err := c.FetchPromptVersion(context.Background(), "sentiment", "^2.0.0", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pd.OutputSchema == nil {
		t.Fatal("expected non-nil OutputSchema")
	}
	typ, _ := pd.OutputSchema["type"].(string)
	if typ != "object" {
		t.Errorf("expected OutputSchema type 'object', got %q", typ)
	}
	props, _ := pd.OutputSchema["properties"].(map[string]interface{})
	if props == nil {
		t.Fatal("expected OutputSchema properties")
	}
	if _, ok := props["sentiment"]; !ok {
		t.Error("expected 'sentiment' in OutputSchema properties")
	}
	if _, ok := props["confidence"]; !ok {
		t.Error("expected 'confidence' in OutputSchema properties")
	}
}

func TestFetchPromptVersion_WithModelConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {
				"prompt": {
					"description": "Prompt with model config",
					"version": {
						"version": "3.0.0",
						"status": "PUBLISHED",
						"metadata": {},
						"outputSchema": null,
						"modelConfig": {
							"provider": "ANTHROPIC",
							"model": "claude-sonnet-4-6",
							"parameters": {"temperature": 0.2, "maxTokens": 1024}
						},
						"files": [
							{
								"name": "userPrompt",
								"content": "Hello",
								"isEntrypoint": true,
								"inputSchema": null,
								"schemaWarnings": []
							}
						]
					}
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	pd, err := c.FetchPromptVersion(context.Background(), "with-model-config", "^3.0.0", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pd.ModelConfig == nil {
		t.Fatal("expected non-nil ModelConfig")
	}
	if pd.ModelConfig["provider"] != "ANTHROPIC" {
		t.Errorf("expected provider ANTHROPIC, got %v", pd.ModelConfig["provider"])
	}
	if pd.ModelConfig["model"] != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %v", pd.ModelConfig["model"])
	}
	params, _ := pd.ModelConfig["parameters"].(map[string]interface{})
	if params == nil {
		t.Fatal("expected ModelConfig parameters")
	}
	if params["maxTokens"] != float64(1024) {
		t.Errorf("expected maxTokens 1024, got %v", params["maxTokens"])
	}
}

func TestFetchPromptVersion_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {"prompt": null},
			"errors": [{"message": "Prompt not found", "path": ["prompt"]}]
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_, err := c.FetchPromptVersion(context.Background(), "nonexistent", "^1.0.0", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- fix: GraphQL errors must render as a plain human message, never the raw
// go-graphql-client Error.Error() dump ("Message: ..., Locations: ...,
// Extensions: map[...], Path: ..."). ---

func TestFetchPromptVersion_ServerError_FriendlyMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": null,
			"errors": [{"message": "Bad Request Exception", "extensions": {"code": "BAD_REQUEST"}}]
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_, err := c.FetchPromptVersion(context.Background(), "broken", "^1.0.0", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertFriendlyGraphQLError(t, err, "Bad Request Exception")
}

func TestListCollectionPrompts_ServerError_FriendlyMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": null,
			"errors": [{"message": "collection not found", "extensions": {"code": "NOT_FOUND"}}]
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_, err := c.ListCollectionPrompts(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertFriendlyGraphQLError(t, err, "collection not found")
}

func TestValidatePrompts_ServerError_FriendlyMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": null,
			"errors": [{"message": "Bad Request Exception", "extensions": {"code": "BAD_REQUEST"}}]
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	err := c.ValidatePrompts(context.Background(), []string{"foo"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertFriendlyGraphQLError(t, err, "Bad Request Exception")
}

// assertFriendlyGraphQLError checks that err carries wantMessage but none of
// the raw go-graphql-client struct dump that used to leak through.
func assertFriendlyGraphQLError(t *testing.T, err error, wantMessage string) {
	t.Helper()
	msg := err.Error()
	if !strings.Contains(msg, wantMessage) {
		t.Errorf("error = %q, want to contain %q", msg, wantMessage)
	}
	for _, leak := range []string{"Extensions:", "Locations:", "Path:", "map[code:"} {
		if strings.Contains(msg, leak) {
			t.Errorf("error = %q, leaks raw GraphQL error internals (%q)", msg, leak)
		}
	}
}

func TestClient_AuthHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got != "Bearer test-api-key" {
			t.Errorf("expected Authorization 'Bearer test-api-key', got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_hello": map[string]any{"name": "hello"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-api-key", "", false)
	_ = c.ValidatePrompts(context.Background(), []string{"hello"})
}

func TestClient_NoAuthHeader_WhenKeyEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.Header["Authorization"]; present {
			t.Errorf("expected no Authorization header, got %q", r.Header.Get("Authorization"))
		}
		if got := r.Header.Get("X-Workspace"); got != "my-workspace" {
			t.Errorf("expected X-Workspace 'my-workspace', got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_hello": map[string]any{"name": "hello"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", "my-workspace", false)
	_ = c.ValidatePrompts(context.Background(), []string{"hello"})
}

func TestClient_XClientHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Client"); got != "cli" {
			t.Errorf("expected X-Client 'cli', got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_hello": map[string]any{"name": "hello"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-api-key", "", false)
	_ = c.ValidatePrompts(context.Background(), []string{"hello"})
}

func TestClient_WorkspaceHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Workspace")
		if got != "my-workspace" {
			t.Errorf("expected X-Workspace 'my-workspace', got %q", got)
		}
		gotAuth := r.Header.Get("Authorization")
		if gotAuth != "Bearer ws-key" {
			t.Errorf("expected Authorization 'Bearer ws-key', got %q", gotAuth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_hello": map[string]any{"name": "hello"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "ws-key", "my-workspace", false)
	_ = c.ValidatePrompts(context.Background(), []string{"hello"})
}

func TestClient_NoWorkspaceHeader_WhenEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Workspace"); got != "" {
			t.Errorf("expected no X-Workspace header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_hello": map[string]any{"name": "hello"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_ = c.ValidatePrompts(context.Background(), []string{"hello"})
}

func TestFetchPromptVersion_WithTools(t *testing.T) {
	var sentQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sentQuery = body.Query

		w.Header().Set("Content-Type", "application/json")
		// Returned out of alias order, and with a cross-workspace pin.
		resp := `{
			"data": {
				"prompt": {
					"description": "Agentic prompt",
					"version": {
						"version": "1.0.0",
						"status": "PUBLISHED",
						"metadata": {},
						"outputSchema": null,
						"modelConfig": null,
						"files": [
							{"name": "main", "content": "Go", "isEntrypoint": true, "inputSchema": null, "schemaWarnings": []}
						],
						"tools": [
							{
								"alias": "web_search",
								"toolVersion": {
									"version": "2.1.0",
									"status": "PUBLISHED",
									"modelDescription": "Searches the web.",
									"inputSchema": {"type": "object", "properties": {"q": {"type": "string"}}},
									"outputSchema": {"type": "object"},
									"metadata": {"owner": "platform"},
									"tool": {"name": "web-search", "workspace": {"name": "vendor"}}
								}
							},
							{
								"alias": "fetch_page",
								"toolVersion": {
									"version": "0.4.0",
									"status": "DRAFT",
									"modelDescription": "Fetches a page.",
									"inputSchema": {"type": "object"},
									"outputSchema": null,
									"metadata": {},
									"tool": {"name": "fetch-page", "workspace": {"name": "acme"}}
								}
							}
						]
					}
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	pd, err := c.FetchPromptVersion(context.Background(), "agent", "*", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// The tool's own workspace has to be selected: a prompt can pin across
	// workspaces, and the ref is what names generated types.
	for _, want := range []string{"tools", "alias", "modelDescription", "workspace"} {
		if !strings.Contains(sentQuery, want) {
			t.Errorf("query is missing %q:\n%s", want, sentQuery)
		}
	}

	if len(pd.Tools) != 2 {
		t.Fatalf("expected 2 pins, got %d", len(pd.Tools))
	}
	// Sorted by alias regardless of the order the server returned them in.
	if pd.Tools[0].Alias != "fetch_page" || pd.Tools[1].Alias != "web_search" {
		t.Errorf("pins are not sorted by alias: %q, %q", pd.Tools[0].Alias, pd.Tools[1].Alias)
	}

	search := pd.Tools[1]
	if search.Ref != "@vendor/web-search" {
		t.Errorf("expected the tool's own workspace in the ref, got %q", search.Ref)
	}
	if search.Version != "2.1.0" || search.Status != "PUBLISHED" {
		t.Errorf("unexpected version/status: %q %q", search.Version, search.Status)
	}
	if search.ModelDescription != "Searches the web." {
		t.Errorf("unexpected model description: %q", search.ModelDescription)
	}
	if search.InputSchema == nil || search.InputSchema["type"] != "object" {
		t.Errorf("input schema did not decode: %v", search.InputSchema)
	}
	if search.OutputSchema == nil {
		t.Error("expected an output schema")
	}
	if search.Metadata["owner"] != "platform" {
		t.Errorf("expected metadata carried through, got %v", search.Metadata)
	}

	draft := pd.Tools[0]
	if draft.Status != "DRAFT" {
		t.Errorf("expected the draft pin's status carried through, got %q", draft.Status)
	}
	if draft.OutputSchema != nil {
		t.Errorf("a null output schema must decode to nil, got %v", draft.OutputSchema)
	}
}

// A version that pins nothing must leave Tools nil, not empty: the cache bytes
// and the integrity hash both depend on it.
func TestFetchPromptVersion_EmptyToolsDecodesToNil(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {
				"prompt": {
					"description": "Plain prompt",
					"version": {
						"version": "1.0.0",
						"status": "PUBLISHED",
						"metadata": {},
						"outputSchema": null,
						"modelConfig": null,
						"files": [
							{"name": "main", "content": "Hi", "isEntrypoint": true, "inputSchema": null, "schemaWarnings": []}
						],
						"tools": []
					}
				}
			}
		}`
		_, _ = w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	pd, err := c.FetchPromptVersion(context.Background(), "plain", "*", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if pd.Tools != nil {
		t.Errorf("expected nil Tools for a prompt that pins none, got %#v", pd.Tools)
	}
}
