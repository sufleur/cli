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

func TestClient_Me_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "me { id email }") {
			t.Errorf("query = %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"me":{"id":"u-1","email":"alice@example.com"}}}`))
	}))
	defer server.Close()

	c := New(server.URL, "u_test", false)
	got, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if got.ID != "u-1" || got.Email != "alice@example.com" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_Me_BearerRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(server.URL, "u_bad", false).Me(context.Background())
	if !errors.Is(err, ErrBearerRejected) {
		t.Errorf("err = %v, want ErrBearerRejected", err)
	}
}

func TestClient_Me_MissingData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"me":null}}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "u_test", false).Me(context.Background())
	if err == nil {
		t.Fatal("expected error for missing me field")
	}
}
