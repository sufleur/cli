package userapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_IngestCases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/dataset/orders/versions/draft/cases" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer u_test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Workspace"); got != "acme" {
			t.Errorf("X-Workspace = %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile(file): %v", err)
		}
		defer file.Close()
		if !strings.HasSuffix(header.Filename, ".jsonl") {
			t.Errorf("filename = %q, want .jsonl suffix", header.Filename)
		}
		content, _ := io.ReadAll(file)
		if !strings.Contains(string(content), `"id":1`) {
			t.Errorf("uploaded content = %q", content)
		}
		_, _ = w.Write([]byte(`{"versionId":"v1","caseCount":2,"byteSize":42,"schema":{"type":"object"},"schemaInferred":true,"enumSuggestions":[{"field":"status","values":["a","b"]}],"validation":{"valid":true,"caseCount":2,"violations":[]}}`))
	}))
	defer server.Close()

	body := strings.NewReader("{\"id\":1}\n{\"id\":2}\n")
	res, err := New(server.URL, "u_test", false).IngestCases(context.Background(), "acme", "orders", "draft", "cases.jsonl", body)
	if err != nil {
		t.Fatalf("IngestCases: %v", err)
	}
	if res.CaseCount != 2 || !res.SchemaInferred {
		t.Errorf("got %+v", res)
	}
	if len(res.EnumSuggestions) != 1 || res.EnumSuggestions[0].Field != "status" {
		t.Errorf("enumSuggestions = %+v", res.EnumSuggestions)
	}
	if res.Validation == nil || !res.Validation.Valid {
		t.Errorf("validation = %+v", res.Validation)
	}
}

func TestClient_IngestCases_BearerRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := New(server.URL, "u_test", false).IngestCases(context.Background(), "acme", "orders", "draft", "cases.jsonl", strings.NewReader("{}"))
	if err != ErrBearerRejected {
		t.Fatalf("err = %v, want ErrBearerRejected", err)
	}
}

func TestClient_DownloadCases(t *testing.T) {
	jsonl := "{\"id\":1}\n{\"id\":2}\n"
	var gzbuf bytes.Buffer
	gw := gzip.NewWriter(&gzbuf)
	_, _ = gw.Write([]byte(jsonl))
	_ = gw.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzbuf.Bytes())
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).DownloadCases(context.Background(), server.URL+"/signed")
	if err != nil {
		t.Fatalf("DownloadCases: %v", err)
	}
	if string(got) != jsonl {
		t.Errorf("got %q, want %q", got, jsonl)
	}
}
