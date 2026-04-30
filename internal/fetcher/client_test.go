package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestValidatePrompts_AllValid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
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
		w.Write([]byte(resp))
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
						"userPromptInputSchema": {"type": "object", "properties": {"name": {"type": "string"}}},
						"systemPromptInputSchema": null,
						"outputSchema": null,
						"files": [
							{"name": "main.txt", "content": "Hello {{name}}"},
							{"name": "system.txt", "content": "You are helpful"}
						]
					}
				}
			}
		}`
		w.Write([]byte(resp))
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
	if pd.Files[0].Name != "main.txt" || pd.Files[0].Content != "Hello {{name}}" {
		t.Errorf("unexpected first file: %+v", pd.Files[0])
	}
	if pd.Files[1].Name != "system.txt" {
		t.Errorf("unexpected second file: %+v", pd.Files[1])
	}
	if pd.UserPromptInputSchema == nil {
		t.Error("expected non-nil UserPromptInputSchema")
	}
	if pd.SystemPromptInputSchema != nil {
		t.Error("expected nil SystemPromptInputSchema")
	}
	if pd.OutputSchema != nil {
		t.Error("expected nil OutputSchema")
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
						"userPromptInputSchema": null,
						"systemPromptInputSchema": null,
						"outputSchema": {
							"type": "object",
							"properties": {
								"sentiment": {"type": "string", "enum": ["positive", "neutral", "negative"]},
								"confidence": {"type": "number"}
							},
							"required": ["sentiment", "confidence"]
						},
						"files": [
							{"name": "userPrompt", "content": "Analyze: {{text}}"}
						]
					}
				}
			}
		}`
		w.Write([]byte(resp))
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

func TestFetchPromptVersion_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{
			"data": {"prompt": null},
			"errors": [{"message": "Prompt not found", "path": ["prompt"]}]
		}`
		w.Write([]byte(resp))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_, err := c.FetchPromptVersion(context.Background(), "nonexistent", "^1.0.0", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_AuthHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got != "Bearer test-api-key" {
			t.Errorf("expected Authorization 'Bearer test-api-key', got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
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
		json.NewEncoder(w).Encode(map[string]any{
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
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt_hello": map[string]any{"name": "hello"},
			},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", "", false)
	_ = c.ValidatePrompts(context.Background(), []string{"hello"})
}
