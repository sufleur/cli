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

func decodeReq(t *testing.T, r *http.Request) graphqlRequest {
	t.Helper()
	body, _ := io.ReadAll(r.Body)
	var req graphqlRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decoding request: %v", err)
	}
	return req
}

func TestClient_ListDatasets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Workspace"); got != "acme" {
			t.Errorf("X-Workspace = %q, want acme", got)
		}
		req := decodeReq(t, r)
		if !strings.Contains(req.Query, "datasets(pagination: $pagination, search: $search)") {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["search"] != "ord" {
			t.Errorf("search = %v", req.Variables["search"])
		}
		_, _ = w.Write([]byte(`{"data":{"datasets":{"total":3,"data":[{"id":"d1","name":"orders","description":"","visibility":"PRIVATE","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-15T09:11:02Z"}]}}}`))
	}))
	defer server.Close()

	page, err := New(server.URL, "u_test", false).ListDatasets(context.Background(), "acme", "ord", 10, 0)
	if err != nil {
		t.Fatalf("ListDatasets: %v", err)
	}
	if page.Total != 3 || len(page.Data) != 1 || page.Data[0].Name != "orders" {
		t.Errorf("got %+v", page)
	}
}

func TestClient_GetDataset_IncludesVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if !strings.Contains(req.Query, "dataset(datasetName: $datasetName)") {
			t.Errorf("query = %q", req.Query)
		}
		if !strings.Contains(req.Query, "versions(pagination: $pagination)") {
			t.Errorf("query missing versions: %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"dataset":{"id":"d1","name":"orders","description":"d","visibility":"PUBLIC","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-15T09:11:02Z","versions":{"total":1,"data":[{"version":"1.0.0","status":"PUBLISHED","caseCount":12,"byteSize":345,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-15T09:11:02Z"}]}}}}`))
	}))
	defer server.Close()

	d, err := New(server.URL, "u_test", false).GetDataset(context.Background(), "acme", "orders")
	if err != nil {
		t.Fatalf("GetDataset: %v", err)
	}
	if d.Versions == nil || len(d.Versions.Data) != 1 || d.Versions.Data[0].CaseCount != 12 {
		t.Errorf("versions = %+v", d.Versions)
	}
}

func TestClient_CreateDataset_OmitsEmptyOptionals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if _, present := req.Variables["description"]; present {
			t.Errorf("description should be absent when empty")
		}
		if _, present := req.Variables["visibility"]; present {
			t.Errorf("visibility should be absent when empty")
		}
		_, _ = w.Write([]byte(`{"data":{"createDataset":{"id":"d1","name":"orders","description":"","visibility":"PRIVATE","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).CreateDataset(context.Background(), "acme", "orders", "", ""); err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}
}

func TestClient_CreateDataset_SendsVisibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if req.Variables["visibility"] != "PUBLIC" {
			t.Errorf("visibility = %v, want PUBLIC", req.Variables["visibility"])
		}
		_, _ = w.Write([]byte(`{"data":{"createDataset":{"id":"d1","name":"orders","description":"","visibility":"PUBLIC","createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).CreateDataset(context.Background(), "acme", "orders", "", "PUBLIC"); err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}
}

func TestClient_GetDatasetVersion_DecodesValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if req.Variables["constraint"] != "draft" {
			t.Errorf("constraint = %v", req.Variables["constraint"])
		}
		_, _ = w.Write([]byte(`{"data":{"dataset":{"version":{"version":"draft","status":"DRAFT","schema":{"type":"object"},"caseCount":2,"byteSize":99,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","validation":{"valid":false,"caseCount":2,"violations":[{"caseIndex":1,"constraint":"/age type","message":"should be number"}]}}}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).GetDatasetVersion(context.Background(), "acme", "orders", "draft")
	if err != nil {
		t.Fatalf("GetDatasetVersion: %v", err)
	}
	if v.Validation == nil || v.Validation.Valid || len(v.Validation.Violations) != 1 {
		t.Fatalf("validation = %+v", v.Validation)
	}
	if v.Validation.Violations[0].CaseIndex != 1 {
		t.Errorf("violation = %+v", v.Validation.Violations[0])
	}
}

func TestClient_SetDatasetSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if !strings.Contains(req.Query, "setDatasetSchema(datasetName: $datasetName, version: $version, schema: $schema)") {
			t.Errorf("query = %q", req.Query)
		}
		schema, ok := req.Variables["schema"].(map[string]any)
		if !ok || schema["type"] != "object" {
			t.Errorf("schema = %+v", req.Variables["schema"])
		}
		_, _ = w.Write([]byte(`{"data":{"setDatasetSchema":{"version":"draft","status":"DRAFT","schema":{"type":"object"},"caseCount":0,"byteSize":0,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","validation":{"valid":true,"caseCount":0,"violations":[]}}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).SetDatasetSchema(context.Background(), "acme", "orders", "draft", map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("SetDatasetSchema: %v", err)
	}
	if v.Validation == nil || !v.Validation.Valid {
		t.Errorf("validation = %+v", v.Validation)
	}
}

func TestClient_PublishDatasetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeReq(t, r)
		if req.Variables["version"] != "1.0.0" {
			t.Errorf("version = %v, want 1.0.0", req.Variables["version"])
		}
		_, _ = w.Write([]byte(`{"data":{"publishDatasetVersion":{"version":"1.0.0","status":"PUBLISHED","schema":{},"caseCount":5,"byteSize":10,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z","validation":{"valid":true,"caseCount":5,"violations":[]}}}}`))
	}))
	defer server.Close()

	v, err := New(server.URL, "u_test", false).PublishDatasetVersion(context.Background(), "acme", "orders", "1.0.0")
	if err != nil {
		t.Fatalf("PublishDatasetVersion: %v", err)
	}
	if v.Version != "1.0.0" || v.Status != "PUBLISHED" {
		t.Errorf("got %+v", v)
	}
}

func TestClient_DeleteDatasetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"deleteDatasetVersion":true}}`))
	}))
	defer server.Close()

	ok, err := New(server.URL, "u_test", false).DeleteDatasetVersion(context.Background(), "acme", "orders", "draft")
	if err != nil {
		t.Fatalf("DeleteDatasetVersion: %v", err)
	}
	if !ok {
		t.Errorf("expected deleted=true")
	}
}
