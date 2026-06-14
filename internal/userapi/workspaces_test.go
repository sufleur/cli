package userapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ListWorkspaces_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Workspace"); got != "" {
			t.Errorf("X-Workspace = %q, want empty (user-scoped)", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "me { memberships { role workspace {") {
			t.Errorf("query = %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"me":{"memberships":[` +
			`{"role":"OWNER","workspace":{"name":"acme","displayName":"Acme Inc","description":"Our prompts"}},` +
			`{"role":"MEMBER","workspace":{"name":"personal","displayName":null,"description":null}}` +
			`]}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d workspaces, want 2: %+v", len(got), got)
	}
	if got[0] != (Workspace{Name: "acme", DisplayName: "Acme Inc", Description: "Our prompts", Role: "OWNER"}) {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1] != (Workspace{Name: "personal", DisplayName: "", Description: "", Role: "MEMBER"}) {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestClient_ListWorkspaces_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"me":{"memberships":[]}}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "u_test", false).ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d workspaces, want 0", len(got))
	}
}

func TestClient_ListWorkspaces_BearerRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(server.URL, "u_bad", false).ListWorkspaces(context.Background())
	if !errors.Is(err, ErrBearerRejected) {
		t.Errorf("err = %v, want ErrBearerRejected", err)
	}
}

func TestClient_ListWorkspaces_MissingData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"me":null}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "u_test", false).ListWorkspaces(context.Background()); err == nil {
		t.Fatal("expected error for missing me field")
	}
}
