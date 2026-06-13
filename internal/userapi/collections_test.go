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

func TestClient_GetCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Workspace"); got != "acme" {
			t.Errorf("X-Workspace = %q, want acme", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "promptCollection(name: $name)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["name"] != "onboarding" {
			t.Errorf("name = %v, want onboarding", req.Variables["name"])
		}
		_, _ = w.Write([]byte(`{"data":{"promptCollection":{"name":"onboarding","description":"d","readme":"# hi","visibility":"PRIVATE","promptCount":3,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-15T09:11:02Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).GetCollection(context.Background(), "acme", "onboarding")
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if got.Name != "onboarding" || got.PromptCount != 3 || got.Readme != "# hi" {
		t.Errorf("got %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps zero: %+v", got)
	}
}

func TestClient_ListCollectionPrompts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "prompts { name }") {
			t.Errorf("query = %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"promptCollection":{"prompts":[{"name":"welcome"},{"name":"goodbye"}]}}}`))
	}))
	defer server.Close()

	names, err := New(server.URL, "u_test", false).ListCollectionPrompts(context.Background(), "acme", "onboarding")
	if err != nil {
		t.Fatalf("ListCollectionPrompts: %v", err)
	}
	if len(names) != 2 || names[0] != "welcome" || names[1] != "goodbye" {
		t.Errorf("got %+v", names)
	}
}

func TestClient_CreateCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "createPromptCollection(name: $name, description: $description)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["name"] != "onboarding" || req.Variables["description"] != "d" {
			t.Errorf("vars = %+v", req.Variables)
		}
		_, _ = w.Write([]byte(`{"data":{"createPromptCollection":{"name":"onboarding","description":"d","readme":null,"visibility":"PRIVATE","promptCount":0,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).CreateCollection(context.Background(), "acme", "onboarding", "d")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if got.Name != "onboarding" || got.Visibility != "PRIVATE" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_CreateCollection_OmitsEmptyDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if _, present := req.Variables["description"]; present {
			t.Errorf("description should be absent when empty")
		}
		_, _ = w.Write([]byte(`{"data":{"createPromptCollection":{"name":"onboarding","description":null,"readme":null,"visibility":"PRIVATE","promptCount":0,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).CreateCollection(context.Background(), "acme", "onboarding", ""); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
}

func TestClient_UpdateCollectionReadme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "updatePromptCollection(name: $name, args: $args)") {
			t.Errorf("query = %q", req.Query)
		}
		args, ok := req.Variables["args"].(map[string]any)
		if !ok {
			t.Fatalf("args not a map: %T", req.Variables["args"])
		}
		if args["readme"] != "# new" {
			t.Errorf("readme = %v", args["readme"])
		}
		if _, present := args["description"]; present {
			t.Errorf("description should be absent on a readme-only update")
		}
		_, _ = w.Write([]byte(`{"data":{"updatePromptCollection":{"name":"onboarding","description":"d","readme":"# new","visibility":"PRIVATE","promptCount":0,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-16T09:11:02Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).UpdateCollectionReadme(context.Background(), "acme", "onboarding", "# new")
	if err != nil {
		t.Fatalf("UpdateCollectionReadme: %v", err)
	}
	if got.Readme != "# new" {
		t.Errorf("readme = %q", got.Readme)
	}
}

func TestClient_UpdateCollectionDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		args, ok := req.Variables["args"].(map[string]any)
		if !ok || args["description"] != "new desc" {
			t.Errorf("args = %+v", req.Variables["args"])
		}
		if _, present := args["readme"]; present {
			t.Errorf("readme should be absent on a description-only update")
		}
		_, _ = w.Write([]byte(`{"data":{"updatePromptCollection":{"name":"onboarding","description":"new desc","readme":null,"visibility":"PRIVATE","promptCount":0,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-16T09:11:02Z"}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).UpdateCollectionDescription(context.Background(), "acme", "onboarding", "new desc")
	if err != nil {
		t.Fatalf("UpdateCollectionDescription: %v", err)
	}
	if got.Description != "new desc" {
		t.Errorf("description = %q", got.Description)
	}
}

func TestClient_SetPromptCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "setPromptCollection(promptName: $promptName, collectionName: $collectionName)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["promptName"] != "welcome" || req.Variables["collectionName"] != "onboarding" {
			t.Errorf("vars = %+v", req.Variables)
		}
		_, _ = w.Write([]byte(`{"data":{"setPromptCollection":{"name":"welcome"}}}`))
	}))
	defer server.Close()

	if err := New(server.URL, "u_test", false).SetPromptCollection(context.Background(), "acme", "welcome", "onboarding"); err != nil {
		t.Fatalf("SetPromptCollection: %v", err)
	}
}

func TestClient_GetPromptCurrentCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"prompt":{"collection":{"name":"onboarding"}}}}`))
	}))
	defer server.Close()

	name, err := New(server.URL, "u_test", false).GetPromptCurrentCollection(context.Background(), "acme", "welcome")
	if err != nil {
		t.Fatalf("GetPromptCurrentCollection: %v", err)
	}
	if name != "onboarding" {
		t.Errorf("name = %q, want onboarding", name)
	}
}

func TestClient_GetPromptCurrentCollection_None(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"prompt":{"collection":null}}}`))
	}))
	defer server.Close()

	name, err := New(server.URL, "u_test", false).GetPromptCurrentCollection(context.Background(), "acme", "welcome")
	if err != nil {
		t.Fatalf("GetPromptCurrentCollection: %v", err)
	}
	if name != "" {
		t.Errorf("name = %q, want empty", name)
	}
}
