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

func TestClient_GetPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Workspace"); got != "acme" {
			t.Errorf("X-Workspace = %q, want acme", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "prompt(promptName: $promptName)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["promptName"] != "welcome" {
			t.Errorf("promptName = %v, want welcome", req.Variables["promptName"])
		}
		_, _ = w.Write([]byte(`{"data":{"prompt":{"name":"welcome","description":"hi","visibility":"PUBLIC","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-15T09:11:02Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).GetPrompt(context.Background(), "acme", "welcome")
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got.Name != "welcome" || got.Description != "hi" || got.Visibility != "PUBLIC" {
		t.Errorf("got %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps zero: %+v", got)
	}
}

func TestClient_ListPrompts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		pag, ok := req.Variables["pagination"].(map[string]any)
		if !ok {
			t.Fatalf("pagination not a map: %T", req.Variables["pagination"])
		}
		if pag["take"] != float64(10) || pag["skip"] != float64(5) {
			t.Errorf("pagination = %+v, want take=10 skip=5", pag)
		}
		if req.Variables["search"] != "wel" {
			t.Errorf("search = %v, want wel", req.Variables["search"])
		}
		_, _ = w.Write([]byte(`{"data":{"prompts":{"total":42,"data":[{"name":"welcome","description":"","visibility":"PUBLIC","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-15T09:11:02Z"}]}}}`))
	}))
	defer server.Close()

	page, err := New(server.URL, "u_test", false).ListPrompts(context.Background(), "acme", "wel", 10, 5)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if page.Total != 42 || len(page.Data) != 1 || page.Data[0].Name != "welcome" {
		t.Errorf("got %+v", page)
	}
}

func TestClient_ListPrompts_OmitsEmptySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if _, present := req.Variables["search"]; present {
			t.Errorf("search should be absent when empty")
		}
		_, _ = w.Write([]byte(`{"data":{"prompts":{"total":0,"data":[]}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).ListPrompts(context.Background(), "acme", "", 10, 0); err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
}

func TestClient_CreatePrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "createPrompt(name: $name, description: $description)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["name"] != "welcome" {
			t.Errorf("name = %v", req.Variables["name"])
		}
		if req.Variables["description"] != "hi there" {
			t.Errorf("description = %v", req.Variables["description"])
		}
		_, _ = w.Write([]byte(`{"data":{"createPrompt":{"name":"welcome","description":"hi there","visibility":"PRIVATE","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).CreatePrompt(context.Background(), "acme", "welcome", "hi there")
	if err != nil {
		t.Fatalf("CreatePrompt: %v", err)
	}
	if got.Name != "welcome" || got.Description != "hi there" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_CreatePrompt_OmitsEmptyDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if _, present := req.Variables["description"]; present {
			t.Errorf("description should be absent when empty")
		}
		_, _ = w.Write([]byte(`{"data":{"createPrompt":{"name":"welcome","description":"","visibility":"PRIVATE","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).CreatePrompt(context.Background(), "acme", "welcome", ""); err != nil {
		t.Fatalf("CreatePrompt: %v", err)
	}
}

func TestClient_UpdatePromptDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "updatePrompt(promptName: $promptName, args: $args)") {
			t.Errorf("query = %q", req.Query)
		}
		args, ok := req.Variables["args"].(map[string]any)
		if !ok || args["description"] != "new desc" {
			t.Errorf("args = %+v", req.Variables["args"])
		}
		_, _ = w.Write([]byte(`{"data":{"updatePrompt":{"name":"welcome","description":"new desc","visibility":"PUBLIC","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-16T09:11:02Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).UpdatePromptDescription(context.Background(), "acme", "welcome", "new desc")
	if err != nil {
		t.Fatalf("UpdatePromptDescription: %v", err)
	}
	if got.Description != "new desc" {
		t.Errorf("description = %q", got.Description)
	}
}
