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

func TestClient_CreatePromptFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "createPromptFile(promptName: $promptName, version: $version, args: $args)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["promptName"] != "welcome" || req.Variables["version"] != "draft" {
			t.Errorf("vars = %+v", req.Variables)
		}
		args, ok := req.Variables["args"].(map[string]any)
		if !ok {
			t.Fatalf("args type: %T", req.Variables["args"])
		}
		if args["name"] != "greeting" || args["content"] != "hi {{name}}" || args["isEntrypoint"] != true {
			t.Errorf("args = %+v", args)
		}
		_, _ = w.Write([]byte(`{"data":{"createPromptFile":{"name":"greeting","content":"hi {{name}}","isEntrypoint":true}}}`))
	}))
	defer server.Close()

	f, err := New(server.URL, "u_test", false).CreatePromptFile(context.Background(), "acme", "welcome", "draft", "greeting", "hi {{name}}", true)
	if err != nil {
		t.Fatalf("CreatePromptFile: %v", err)
	}
	if f.Name != "greeting" || !f.IsEntrypoint {
		t.Errorf("got %+v", f)
	}
}

func TestClient_UpdatePromptFile_OmitsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		args, ok := req.Variables["args"].(map[string]any)
		if !ok {
			t.Fatalf("args: %v", req.Variables["args"])
		}
		// Only content set; name absent.
		if args["content"] != "new" {
			t.Errorf("content = %v", args["content"])
		}
		if _, present := args["name"]; present {
			t.Errorf("name should be absent when rename empty")
		}
		_, _ = w.Write([]byte(`{"data":{"updatePromptFile":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[{"name":"greeting","content":"new","isEntrypoint":false}]}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).UpdatePromptFile(context.Background(), "acme", "welcome", "draft", "greeting", "new", "")
	if err != nil {
		t.Fatalf("UpdatePromptFile: %v", err)
	}
	if len(v.Files) != 1 || v.Files[0].Content != "new" {
		t.Errorf("files = %+v", v.Files)
	}
}

func TestClient_UpdatePromptFile_RenameOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		args := req.Variables["args"].(map[string]any)
		if args["name"] != "farewell" {
			t.Errorf("name = %v, want farewell", args["name"])
		}
		if _, present := args["content"]; present {
			t.Errorf("content should be absent when only renaming")
		}
		_, _ = w.Write([]byte(`{"data":{"updatePromptFile":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[{"name":"farewell","content":"bye","isEntrypoint":false}]}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).UpdatePromptFile(context.Background(), "acme", "welcome", "draft", "greeting", "", "farewell"); err != nil {
		t.Fatalf("UpdatePromptFile: %v", err)
	}
}

func TestClient_DeletePromptFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if req.Variables["fileName"] != "greeting" {
			t.Errorf("fileName = %v", req.Variables["fileName"])
		}
		_, _ = w.Write([]byte(`{"data":{"deletePromptFile":true}}`))
	}))
	defer server.Close()

	ok, err := New(server.URL, "u_test", false).DeletePromptFile(context.Background(), "acme", "welcome", "draft", "greeting")
	if err != nil {
		t.Fatalf("DeletePromptFile: %v", err)
	}
	if !ok {
		t.Error("ok = false, want true")
	}
}

func TestClient_SetPromptFileEntrypoint(t *testing.T) {
	for _, val := range []bool{true, false} {
		t.Run(map[bool]string{true: "set", false: "clear"}[val], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req graphqlRequest
				_ = json.Unmarshal(body, &req)
				if req.Variables["isEntrypoint"] != val {
					t.Errorf("isEntrypoint = %v, want %v", req.Variables["isEntrypoint"], val)
				}
				_, _ = w.Write([]byte(`{"data":{"setPromptFileEntrypoint":{"version":"1.0.0","status":"DRAFT","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","metadata":{},"outputSchema":null,"files":[{"name":"greeting","content":"","isEntrypoint":` +
					map[bool]string{true: "true", false: "false"}[val] + `}]}}}`))
			}))
			defer server.Close()

			v, err := New(server.URL, "u_test", false).SetPromptFileEntrypoint(context.Background(), "acme", "welcome", "draft", "greeting", val)
			if err != nil {
				t.Fatalf("SetPromptFileEntrypoint: %v", err)
			}
			if len(v.Files) != 1 || v.Files[0].IsEntrypoint != val {
				t.Errorf("files = %+v", v.Files)
			}
		})
	}
}
