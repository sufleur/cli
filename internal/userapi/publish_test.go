package userapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_PublishPromptVersion(t *testing.T) {
	var req graphqlRequest
	var workspace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req = decodeReq(t, r)
		workspace = r.Header.Get("X-Workspace")
		_, _ = w.Write([]byte(`{"data":{"publishPromptVersion":{"version":"1.2.0","status":"PUBLISHED"}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).PublishPromptVersion(context.Background(), "acme", "agent", "1.2.0")
	if err != nil {
		t.Fatalf("PublishPromptVersion: %v", err)
	}
	if !strings.Contains(req.Query, "publishPromptVersion(promptName: $promptName, version: $version)") ||
		!strings.Contains(req.Query, "$promptName: ID!") || !strings.Contains(req.Query, "$version: ID!") {
		t.Errorf("unexpected query: %q", req.Query)
	}
	if req.Variables["promptName"] != "agent" || req.Variables["version"] != "1.2.0" {
		t.Errorf("variables = %v", req.Variables)
	}
	if workspace != "acme" {
		t.Errorf("X-Workspace = %q", workspace)
	}
	if v.Version != "1.2.0" || v.Status != "PUBLISHED" {
		t.Errorf("decoded = %+v", v)
	}
}

func TestClient_PublishToolVersion_SendsVersionAsString(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"publishToolVersion":{"version":"2.0.0","status":"PUBLISHED"}}}`, &req)
	defer server.Close()

	v, err := New(server.URL, "u_test", false).PublishToolVersion(context.Background(), "acme", "web-search", "2.0.0")
	if err != nil {
		t.Fatalf("PublishToolVersion: %v", err)
	}
	if !strings.Contains(req.Query, "publishToolVersion(toolName: $toolName, version: $version)") ||
		!strings.Contains(req.Query, "$toolName: ID!") || !strings.Contains(req.Query, "$version: String!") {
		t.Errorf("unexpected query: %q", req.Query)
	}
	if req.Variables["toolName"] != "web-search" || req.Variables["version"] != "2.0.0" {
		t.Errorf("variables = %v", req.Variables)
	}
	if v.Version != "2.0.0" {
		t.Errorf("decoded = %+v", v)
	}
}

func TestClient_PublishDatasetVersion(t *testing.T) {
	var req graphqlRequest
	server := toolServer(t, `{"data":{"publishDatasetVersion":{"version":"1.0.0","status":"PUBLISHED","caseCount":7}}}`, &req)
	defer server.Close()

	v, err := New(server.URL, "u_test", false).PublishDatasetVersion(context.Background(), "acme", "cases", "1.0.0")
	if err != nil {
		t.Fatalf("PublishDatasetVersion: %v", err)
	}
	if !strings.Contains(req.Query, "publishDatasetVersion(datasetName: $datasetName, version: $version)") ||
		!strings.Contains(req.Query, "$datasetName: ID!") || !strings.Contains(req.Query, "$version: ID!") {
		t.Errorf("unexpected query: %q", req.Query)
	}
	if req.Variables["datasetName"] != "cases" || req.Variables["version"] != "1.0.0" {
		t.Errorf("variables = %v", req.Variables)
	}
	if v.Version != "1.0.0" || v.CaseCount != 7 {
		t.Errorf("decoded = %+v", v)
	}
}

func TestDo_GraphQLErrorCarriesExtensionCode(t *testing.T) {
	server := toolServer(t, `{"data":null,"errors":[{"message":"publishing is off","extensions":{"code":"CLI_PUBLISHING_DISABLED"}}]}`, nil)
	defer server.Close()

	err := New(server.URL, "u_test", false).Do(context.Background(), Request{Query: "query { x }"}, nil)
	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) {
		t.Fatalf("expected *GraphQLError, got %T: %v", err, err)
	}
	if gqlErr.Code != CodeCLIPublishingDisabled {
		t.Errorf("code = %q", gqlErr.Code)
	}
	if CodeCLIPublishingDisabled != "CLI_PUBLISHING_DISABLED" {
		t.Errorf("wire value changed: %q", CodeCLIPublishingDisabled)
	}
	if gqlErr.Message != "publishing is off" {
		t.Errorf("message = %q", gqlErr.Message)
	}
	if err.Error() != "graphql error: publishing is off" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestDo_GraphQLErrorWithoutExtensionsKeepsPlainShape(t *testing.T) {
	server := toolServer(t, `{"data":null,"errors":[{"message":"nope"}]}`, nil)
	defer server.Close()

	err := New(server.URL, "u_test", false).Do(context.Background(), Request{Query: "query { x }"}, nil)
	if err == nil || err.Error() != "graphql error: nope" {
		t.Fatalf("err = %v", err)
	}
	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) || gqlErr.Code != "" {
		t.Errorf("expected empty code, got %+v", gqlErr)
	}
}
