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

func TestDo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("path = %q, want /graphql", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer u_test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Client"); got != "cli" {
			t.Errorf("X-Client = %q, want cli", got)
		}
		if got := r.Header.Get("X-Workspace"); got != "acme" {
			t.Errorf("X-Workspace = %q, want acme", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if req.Query != "query { thing }" {
			t.Errorf("query = %q", req.Query)
		}
		if req.Variables["x"] != float64(1) {
			t.Errorf("variables[x] = %v", req.Variables["x"])
		}
		_, _ = w.Write([]byte(`{"data":{"thing":{"id":"t1","name":"hello"}}}`))
	}))
	defer server.Close()

	c := New(server.URL, "u_test", false)
	var got struct {
		Thing struct {
			ID, Name string
		} `json:"thing"`
	}
	err := c.Do(context.Background(), Request{
		Query:     "query { thing }",
		Variables: map[string]any{"x": 1},
		Workspace: "acme",
	}, &got)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got.Thing.ID != "t1" || got.Thing.Name != "hello" {
		t.Errorf("got %+v", got)
	}
}

func TestDo_OmitsWorkspaceWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Workspace"); got != "" {
			t.Errorf("X-Workspace = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()

	c := New(server.URL, "u_test", false)
	if err := c.Do(context.Background(), Request{Query: "query { me { id } }"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestDo_BearerRejected(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		err := New(server.URL, "u_test", false).Do(context.Background(), Request{Query: "query { x }"}, nil)
		server.Close()
		if !errors.Is(err, ErrBearerRejected) {
			t.Errorf("status %d: err = %v, want ErrBearerRejected", code, err)
		}
	}
}

func TestDo_GraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"nope"}]}`))
	}))
	defer server.Close()

	c := New(server.URL, "u_test", false)
	err := c.Do(context.Background(), Request{Query: "query { x }"}, nil)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v", err)
	}
}

func TestDo_NilResultIgnoresData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"anything":42}}`))
	}))
	defer server.Close()

	c := New(server.URL, "u_test", false)
	if err := c.Do(context.Background(), Request{Query: "mutation { x }"}, nil); err != nil {
		t.Errorf("Do with nil result: %v", err)
	}
}

func TestDo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer server.Close()

	c := New(server.URL, "u_test", false)
	err := c.Do(context.Background(), Request{Query: "query { x }"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err should mention 500: %v", err)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := New("https://api.example.com/", "u_test", false)
	if c.base != "https://api.example.com" {
		t.Errorf("base = %q", c.base)
	}
}
