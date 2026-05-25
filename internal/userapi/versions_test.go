package userapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_CreatePromptVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "createPromptVersion(promptName: $promptName)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["promptName"] != "welcome" {
			t.Errorf("promptName = %v", req.Variables["promptName"])
		}
		_, _ = w.Write([]byte(`{"data":{"createPromptVersion":{"version":"1.2.4-draft.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[]}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).CreatePromptVersion(context.Background(), "acme", "welcome")
	if err != nil {
		t.Fatalf("CreatePromptVersion: %v", err)
	}
	if v.Version != "1.2.4-draft.0" || v.Status != "DRAFT" {
		t.Errorf("got %+v", v)
	}
}

func TestClient_DeletePromptVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "deletePromptVersion") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["version"] != "1.2.4-draft.0" {
			t.Errorf("version = %v", req.Variables["version"])
		}
		_, _ = w.Write([]byte(`{"data":{"deletePromptVersion":true}}`))
	}))
	defer server.Close()

	ok, err := New(server.URL, "u_test", false).DeletePromptVersion(context.Background(), "acme", "welcome", "1.2.4-draft.0")
	if err != nil {
		t.Fatalf("DeletePromptVersion: %v", err)
	}
	if !ok {
		t.Error("ok = false, want true")
	}
}

func TestClient_GetPromptVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"prompt":{"version":{"version":"1.0.0","status":"PUBLISHED","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{"model":"claude","temperature":0.7},"outputSchema":{"type":"object"},"files":[{"name":"prompt.mustache","content":"Hello {{name}}","isEntrypoint":true}]}}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).GetPromptVersion(context.Background(), "acme", "welcome", "1.0.0")
	if err != nil {
		t.Fatalf("GetPromptVersion: %v", err)
	}
	if v.Version != "1.0.0" || v.Metadata["model"] != "claude" {
		t.Errorf("got %+v", v)
	}
	if len(v.Files) != 1 || v.Files[0].Content != "Hello {{name}}" {
		t.Errorf("files = %+v", v.Files)
	}
	if v.OutputSchema["type"] != "object" {
		t.Errorf("outputSchema = %+v", v.OutputSchema)
	}
}

func TestClient_GetPromptVersion_Missing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"prompt":{"version":null}}}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "u_test", false).GetPromptVersion(context.Background(), "acme", "welcome", "9.9.9")
	if err == nil {
		t.Fatal("expected error when version missing")
	}
}

func TestClient_ListPromptVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if req.Variables["status"] != "DRAFT" {
			t.Errorf("status = %v, want DRAFT", req.Variables["status"])
		}
		_, _ = w.Write([]byte(`{"data":{"prompt":{"versions":{"total":3,"data":[{"version":"1.0.0","status":"PUBLISHED","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[]}]}}}}`))
	}))
	defer server.Close()

	page, err := New(server.URL, "u_test", false).ListPromptVersions(context.Background(), "acme", "welcome", "DRAFT", 50, 0)
	if err != nil {
		t.Fatalf("ListPromptVersions: %v", err)
	}
	if page.Total != 3 || len(page.Data) != 1 {
		t.Errorf("got %+v", page)
	}
}

func TestClient_ListPromptVersions_OmitsEmptyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if _, ok := req.Variables["status"]; ok {
			t.Errorf("status should be absent when empty")
		}
		_, _ = w.Write([]byte(`{"data":{"prompt":{"versions":{"total":0,"data":[]}}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).ListPromptVersions(context.Background(), "acme", "welcome", "", 50, 0); err != nil {
		t.Fatalf("ListPromptVersions: %v", err)
	}
}

func TestClient_SetMetadataVariants(t *testing.T) {
	cases := []struct {
		name        string
		mutation    string
		invoke      func(c *Client) (*PromptVersion, error)
		wantValue   any
		wantQueryEl string
	}{
		{"string", "promptVersionSetStringMetadata", func(c *Client) (*PromptVersion, error) {
			return c.SetPromptVersionStringMetadata(context.Background(), "acme", "welcome", "1.0.0", "model", "claude")
		}, "claude", "$metadataValue: String!"},
		{"integer", "promptVersionSetIntegerMetadata", func(c *Client) (*PromptVersion, error) {
			return c.SetPromptVersionIntegerMetadata(context.Background(), "acme", "welcome", "1.0.0", "count", 10)
		}, float64(10), "$metadataValue: Int!"},
		{"float", "promptVersionSetFloatMetadata", func(c *Client) (*PromptVersion, error) {
			return c.SetPromptVersionFloatMetadata(context.Background(), "acme", "welcome", "1.0.0", "temperature", 0.7)
		}, 0.7, "$metadataValue: Float!"},
		{"boolean", "promptVersionSetBooleanMetadata", func(c *Client) (*PromptVersion, error) {
			return c.SetPromptVersionBooleanMetadata(context.Background(), "acme", "welcome", "1.0.0", "streaming", true)
		}, true, "$metadataValue: Boolean!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req graphqlRequest
				_ = json.Unmarshal(body, &req)
				if !strings.Contains(req.Query, tc.mutation) {
					t.Errorf("query missing %s: %q", tc.mutation, req.Query)
				}
				if !strings.Contains(req.Query, tc.wantQueryEl) {
					t.Errorf("query missing %s: %q", tc.wantQueryEl, req.Query)
				}
				if req.Variables["metadataValue"] != tc.wantValue {
					t.Errorf("metadataValue = %v (%T), want %v (%T)", req.Variables["metadataValue"], req.Variables["metadataValue"], tc.wantValue, tc.wantValue)
				}
				_, _ = w.Write([]byte(`{"data":{"` + tc.mutation + `":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[]}}}`))
			}))
			defer server.Close()

			c := New(server.URL, "u_test", false)
			if _, err := tc.invoke(c); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		})
	}
}

func TestClient_DeletePromptVersionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if req.Variables["metadataKey"] != "model" {
			t.Errorf("metadataKey = %v", req.Variables["metadataKey"])
		}
		_, _ = w.Write([]byte(`{"data":{"promptVersionDeleteMetadata":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[]}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).DeletePromptVersionMetadata(context.Background(), "acme", "welcome", "1.0.0", "model"); err != nil {
		t.Fatalf("DeletePromptVersionMetadata: %v", err)
	}
}

func TestClient_SetPromptVersionOutputSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		schema, ok := req.Variables["outputSchema"].(map[string]any)
		if !ok || schema["type"] != "object" {
			t.Errorf("outputSchema = %+v", req.Variables["outputSchema"])
		}
		_, _ = w.Write([]byte(`{"data":{"promptVersionSetOutputSchema":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":{"type":"object"},"files":[]}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).SetPromptVersionOutputSchema(context.Background(), "acme", "welcome", "1.0.0", map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("SetPromptVersionOutputSchema: %v", err)
	}
	if v.OutputSchema["type"] != "object" {
		t.Errorf("outputSchema not echoed: %+v", v.OutputSchema)
	}
}

func TestClient_SetPromptVersionReadme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "promptVersionSetReadme") {
			t.Errorf("query missing promptVersionSetReadme: %q", req.Query)
		}
		if !strings.Contains(req.Query, "$readme: String!") {
			t.Errorf("query missing $readme: String!: %q", req.Query)
		}
		if req.Variables["readme"] != "# Hello" {
			t.Errorf("readme = %v, want %q", req.Variables["readme"], "# Hello")
		}
		_, _ = w.Write([]byte(`{"data":{"promptVersionSetReadme":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"readme":"# Hello","files":[]}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).SetPromptVersionReadme(context.Background(), "acme", "welcome", "1.0.0", "# Hello")
	if err != nil {
		t.Fatalf("SetPromptVersionReadme: %v", err)
	}
	if v.Readme != "# Hello" {
		t.Errorf("readme not echoed: %q", v.Readme)
	}
}
