package userapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// toolServer answers one request, capturing it for assertions.
func toolServer(t *testing.T, body string, capture *graphqlRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if capture != nil {
			*capture = req
		}
		_, _ = w.Write([]byte(body))
	}))
}

func TestClient_ListTools(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"tools":{"total":2,"data":[
		{"id":"1","name":"web-search","description":"Searches","visibility":"PUBLIC","tags":["search"]},
		{"id":"2","name":"fetch-page","description":"","visibility":"PRIVATE","tags":[]}
	]}}}`, &req)
	defer server.Close()

	page, err := New(server.URL, "u_test", false).ListTools(context.Background(), "acme", "sea", 10, 0)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if !strings.Contains(req.Query, "tools(pagination: $pagination, search: $search)") {
		t.Errorf("query = %q", req.Query)
	}
	if req.Variables["search"] != "sea" {
		t.Errorf("search = %v", req.Variables["search"])
	}
	if page.Total != 2 || len(page.Data) != 2 || page.Data[0].Name != "web-search" {
		t.Errorf("unexpected page: %+v", page)
	}
}

func TestClient_ListTools_OmitsEmptySearch(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"tools":{"total":0,"data":[]}}}`, &req)
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).ListTools(context.Background(), "acme", "", 10, 0); err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if _, present := req.Variables["search"]; present {
		t.Error("an empty search must not be sent as a variable")
	}
}

func TestClient_GetTool(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"tool":{
		"id":"1","name":"web-search","description":"Searches","visibility":"PRIVATE","tags":[],
		"dependentCount":3,
		"versions":{"total":2,"data":[{"version":"1.0.0","status":"PUBLISHED"},{"version":"draft","status":"DRAFT"}]}
	}}}`, &req)
	defer server.Close()

	tool, err := New(server.URL, "u_test", false).GetTool(context.Background(), "acme", "web-search")
	if err != nil {
		t.Fatalf("GetTool: %v", err)
	}
	if !strings.Contains(req.Query, "dependentCount") {
		t.Errorf("query should ask for the dependent count: %q", req.Query)
	}
	if tool.DependentCount == nil || *tool.DependentCount != 3 {
		t.Errorf("dependentCount = %v", tool.DependentCount)
	}
	if tool.Versions == nil || tool.Versions.Total != 2 {
		t.Errorf("versions = %+v", tool.Versions)
	}
}

func TestClient_GetTool_MissingData(t *testing.T) {
	server := toolServer(t, `{"data":{"tool":null}}`, nil)
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).GetTool(context.Background(), "acme", "nope"); err == nil {
		t.Error("expected an error when the tool is absent")
	}
}

func TestClient_CreateTool(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"createTool":{"id":"1","name":"web-search","description":"Searches","visibility":"PRIVATE","tags":[]}}}`, &req)
	defer server.Close()

	tool, err := New(server.URL, "u_test", false).CreateTool(context.Background(), "acme", "web-search", "Searches")
	if err != nil {
		t.Fatalf("CreateTool: %v", err)
	}
	if !strings.Contains(req.Query, "createTool(name: $name, description: $description)") {
		t.Errorf("query = %q", req.Query)
	}
	if req.Variables["name"] != "web-search" || req.Variables["description"] != "Searches" {
		t.Errorf("variables = %v", req.Variables)
	}
	// New tools are private; making one public is web-app-only.
	if tool.Visibility != "PRIVATE" {
		t.Errorf("visibility = %q", tool.Visibility)
	}
}

func TestClient_CreateTool_OmitsEmptyDescription(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"createTool":{"id":"1","name":"t","description":"","visibility":"PRIVATE","tags":[]}}}`, &req)
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).CreateTool(context.Background(), "acme", "t", ""); err != nil {
		t.Fatalf("CreateTool: %v", err)
	}
	if _, present := req.Variables["description"]; present {
		t.Error("an empty description must not be sent")
	}
}

func TestClient_UpdateTool(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"updateTool":{"id":"1","name":"t","description":"New","visibility":"PRIVATE","tags":[]}}}`, &req)
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).UpdateTool(context.Background(), "acme", "t", "New"); err != nil {
		t.Fatalf("UpdateTool: %v", err)
	}
	args, _ := req.Variables["args"].(map[string]any)
	if args["description"] != "New" {
		t.Errorf("args = %v", req.Variables["args"])
	}
}

func TestClient_GetToolVersion(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"tool":{"version":{
		"version":"1.2.0","status":"PUBLISHED","modelDescription":"Searches the web.",
		"inputSchema":{"type":"object"},"outputSchema":null,"metadata":{},"readme":"# Hi"
	}}}}`, &req)
	defer server.Close()

	v, err := New(server.URL, "u_test", false).GetToolVersion(context.Background(), "acme", "web-search", "^1.0.0")
	if err != nil {
		t.Fatalf("GetToolVersion: %v", err)
	}
	if req.Variables["constraint"] != "^1.0.0" {
		t.Errorf("constraint = %v", req.Variables["constraint"])
	}
	if v.ModelDescription != "Searches the web." {
		t.Errorf("modelDescription = %q", v.ModelDescription)
	}
	// A null output schema must be distinguishable from an empty object.
	if v.OutputSchema != nil {
		t.Errorf("outputSchema = %v, want nil", v.OutputSchema)
	}
}

func TestClient_GetToolVersion_NoMatch(t *testing.T) {
	server := toolServer(t, `{"data":{"tool":{"version":null}}}`, nil)
	defer server.Close()

	_, err := New(server.URL, "u_test", false).GetToolVersion(context.Background(), "acme", "web-search", "^9.0.0")
	if err == nil {
		t.Fatal("expected an error when no version matches")
	}
	if !strings.Contains(err.Error(), "^9.0.0") {
		t.Errorf("error should name the constraint: %v", err)
	}
}

func TestClient_ListToolVersions_StatusIsOptional(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"tool":{"versions":{"total":1,"data":[{"version":"1.0.0","status":"PUBLISHED"}]}}}}`, &req)
	defer server.Close()
	client := New(server.URL, "u_test", false)

	if _, err := client.ListToolVersions(context.Background(), "acme", "t", "", 50, 0); err != nil {
		t.Fatalf("ListToolVersions: %v", err)
	}
	// `status` still appears in the selection set; what must be absent is the
	// argument and its variable declaration.
	if strings.Contains(req.Query, "status: $status") || strings.Contains(req.Query, "$status:") {
		t.Errorf("the status argument must not appear when unset: %q", req.Query)
	}

	if _, err := client.ListToolVersions(context.Background(), "acme", "t", "DRAFT", 50, 0); err != nil {
		t.Fatalf("ListToolVersions: %v", err)
	}
	if !strings.Contains(req.Query, "$status: ToolVersionStatus") {
		t.Errorf("status must be declared when set: %q", req.Query)
	}
	if req.Variables["status"] != "DRAFT" {
		t.Errorf("status = %v", req.Variables["status"])
	}
}

func TestClient_CreateToolVersionDraft(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"createToolVersionDraft":{"version":"draft","status":"DRAFT","modelDescription":"","inputSchema":{},"metadata":{},"readme":""}}}`, &req)
	defer server.Close()

	v, err := New(server.URL, "u_test", false).CreateToolVersionDraft(context.Background(), "acme", "t")
	if err != nil {
		t.Fatalf("CreateToolVersionDraft: %v", err)
	}
	if !strings.Contains(req.Query, "createToolVersionDraft(toolName: $toolName)") {
		t.Errorf("query = %q", req.Query)
	}
	if v.Status != "DRAFT" {
		t.Errorf("status = %q", v.Status)
	}
}

func TestClient_DeleteToolVersion(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"deleteToolVersion":true}}`, &req)
	defer server.Close()

	if err := New(server.URL, "u_test", false).DeleteToolVersion(context.Background(), "acme", "t", "draft"); err != nil {
		t.Fatalf("DeleteToolVersion: %v", err)
	}
	if req.Variables["version"] != "draft" {
		t.Errorf("version = %v", req.Variables["version"])
	}
}

func TestClient_DeleteToolVersion_FalseIsAnError(t *testing.T) {
	server := toolServer(t, `{"data":{"deleteToolVersion":false}}`, nil)
	defer server.Close()

	if err := New(server.URL, "u_test", false).DeleteToolVersion(context.Background(), "acme", "t", "draft"); err == nil {
		t.Error("expected an error when the server reports no deletion")
	}
}

// Every setter funnels through one mutation, sending only the key it owns, so
// a partial update never clobbers a sibling field.
func TestClient_ToolVersionSetters_SendOnlyTheirOwnKey(t *testing.T) {
	cases := map[string]struct {
		call func(c *Client) error
		key  string
		want any
	}{
		"modelDescription": {
			call: func(c *Client) error {
				_, err := c.SetToolVersionModelDescription(context.Background(), "acme", "t", "draft", "Searches.")
				return err
			},
			key: "modelDescription", want: "Searches.",
		},
		"inputSchema": {
			call: func(c *Client) error {
				_, err := c.SetToolVersionInputSchema(context.Background(), "acme", "t", "draft", map[string]any{"type": "object"})
				return err
			},
			key: "inputSchema", want: map[string]any{"type": "object"},
		},
		"readme": {
			call: func(c *Client) error {
				_, err := c.SetToolVersionReadme(context.Background(), "acme", "t", "draft", "# Hi")
				return err
			},
			key: "readme", want: "# Hi",
		},
		"metadata": {
			call: func(c *Client) error {
				_, err := c.SetToolVersionMetadata(context.Background(), "acme", "t", "draft", map[string]any{"owner": "platform"})
				return err
			},
			key: "metadata", want: map[string]any{"owner": "platform"},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var req graphqlRequest
			server := toolServer(t, `{"data":{"updateToolVersion":{"version":"draft","status":"DRAFT","modelDescription":"","inputSchema":{},"metadata":{},"readme":""}}}`, &req)
			defer server.Close()

			if err := c.call(New(server.URL, "u_test", false)); err != nil {
				t.Fatalf("setter: %v", err)
			}
			input, _ := req.Variables["input"].(map[string]any)
			if len(input) != 1 {
				t.Fatalf("expected exactly one key in input, got %v", input)
			}
			if _, ok := input[c.key]; !ok {
				t.Errorf("expected key %q, got %v", c.key, input)
			}
		})
	}
}

// Clearing an output schema is a real operation, so nil has to reach the wire
// rather than being dropped as "no change".
func TestClient_SetToolVersionOutputSchema_NilClears(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"updateToolVersion":{"version":"draft","status":"DRAFT","modelDescription":"","inputSchema":{},"metadata":{},"readme":""}}}`, &req)
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).SetToolVersionOutputSchema(context.Background(), "acme", "t", "draft", nil); err != nil {
		t.Fatalf("SetToolVersionOutputSchema: %v", err)
	}
	input, _ := req.Variables["input"].(map[string]any)
	value, present := input["outputSchema"]
	if !present {
		t.Fatal("outputSchema must be sent so the server clears it")
	}
	if value != nil {
		t.Errorf("outputSchema = %v, want null", value)
	}
}
