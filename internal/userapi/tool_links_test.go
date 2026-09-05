package userapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_GetPromptVersionTools(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"prompt":{"version":{"tools":[
		{"alias":"web_search","toolVersion":{"version":"1.2.0","status":"PUBLISHED","modelDescription":"Searches.",
		 "tool":{"name":"web-search","workspace":{"name":"vendor"}}}}
	]}}}}`, &req)
	defer server.Close()

	pins, err := New(server.URL, "u_test", false).GetPromptVersionTools(context.Background(), "acme", "agent", "draft")
	if err != nil {
		t.Fatalf("GetPromptVersionTools: %v", err)
	}
	// The tool's own workspace has to be selected: a prompt can pin across
	// workspaces, so a bare name does not identify the contract.
	if !strings.Contains(req.Query, "workspace { name }") {
		t.Errorf("query must select the tool's workspace: %q", req.Query)
	}
	if len(pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(pins))
	}
	if pins[0].Alias != "web_search" {
		t.Errorf("alias = %q", pins[0].Alias)
	}
	if got := pins[0].ToolVersion.Tool.Ref(); got != "@vendor/web-search" {
		t.Errorf("ref = %q", got)
	}
}

func TestClient_GetPromptVersionTools_NoVersion(t *testing.T) {
	server := toolServer(t, `{"data":{"prompt":{"version":null}}}`, nil)
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).GetPromptVersionTools(context.Background(), "acme", "agent", "9.9.9"); err == nil {
		t.Error("expected an error when the version does not resolve")
	}
}

func TestClient_LinkTool(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"linkTool":{"version":"draft"}}}`, &req)
	defer server.Close()

	err := New(server.URL, "u_test", false).LinkTool(context.Background(),
		"acme", "agent", "draft", "vendor", "web-search", "^1.0.0", "search")
	if err != nil {
		t.Fatalf("LinkTool: %v", err)
	}

	args, _ := req.Variables["args"].(map[string]any)
	tool, _ := args["tool"].(map[string]any)
	if tool["workspace"] != "vendor" || tool["name"] != "web-search" {
		t.Errorf("tool = %v", tool)
	}
	// The constraint goes to the server, which resolves it once and stores a
	// concrete version.
	if tool["version"] != "^1.0.0" {
		t.Errorf("version = %v", tool["version"])
	}
	if args["alias"] != "search" {
		t.Errorf("alias = %v", args["alias"])
	}
}

// An empty alias must be omitted so the server defaults it to the tool's name,
// rather than sent as "" and rejected.
func TestClient_LinkTool_OmitsEmptyAlias(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"linkTool":{"version":"draft"}}}`, &req)
	defer server.Close()

	if err := New(server.URL, "u_test", false).LinkTool(context.Background(),
		"acme", "agent", "draft", "vendor", "web-search", "*", ""); err != nil {
		t.Fatalf("LinkTool: %v", err)
	}
	args, _ := req.Variables["args"].(map[string]any)
	if _, present := args["alias"]; present {
		t.Error("an empty alias must not be sent")
	}
}

func TestClient_UpdateToolLink(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"updateToolLink":{"version":"draft"}}}`, &req)
	defer server.Close()

	if err := New(server.URL, "u_test", false).UpdateToolLink(context.Background(),
		"acme", "agent", "draft", "vendor", "web-search", "lookup"); err != nil {
		t.Fatalf("UpdateToolLink: %v", err)
	}
	tool, _ := req.Variables["tool"].(map[string]any)
	// No version: a prompt version pins at most one version of a given tool, so
	// the tool alone identifies the pin.
	if _, present := tool["version"]; present {
		t.Errorf("rename must not carry a version: %v", tool)
	}
	if req.Variables["alias"] != "lookup" {
		t.Errorf("alias = %v", req.Variables["alias"])
	}
}

func TestClient_UnlinkTool(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"unlinkTool":{"version":"draft"}}}`, &req)
	defer server.Close()

	if err := New(server.URL, "u_test", false).UnlinkTool(context.Background(),
		"acme", "agent", "draft", "vendor", "web-search"); err != nil {
		t.Fatalf("UnlinkTool: %v", err)
	}
	if !strings.Contains(req.Query, "unlinkTool(promptName: $promptName, version: $version, tool: $tool)") {
		t.Errorf("query = %q", req.Query)
	}
}

// A refusal from the server must surface, not be swallowed by the nil result.
func TestClient_LinkTool_SurfacesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"Cannot change the tool dependencies of a published prompt version"}]}`))
	}))
	defer server.Close()

	err := New(server.URL, "u_test", false).LinkTool(context.Background(),
		"acme", "agent", "1.0.0", "vendor", "web-search", "*", "")
	if err == nil || !strings.Contains(err.Error(), "published prompt version") {
		t.Errorf("expected the server's refusal to surface, got %v", err)
	}
}
