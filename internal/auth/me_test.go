package auth

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

func TestFetchMe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer u_test" {
			t.Errorf("Authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "me { id email }") {
			t.Errorf("unexpected query: %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"me":{"id":"u-1","email":"alice@example.com"}}}`))
	}))
	defer server.Close()

	got, err := FetchMe(context.Background(), http.DefaultClient, server.URL, "u_test")
	if err != nil {
		t.Fatalf("FetchMe: %v", err)
	}
	if got.ID != "u-1" || got.Email != "alice@example.com" {
		t.Errorf("got %+v", got)
	}
}

func TestFetchMe_BearerRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := FetchMe(context.Background(), http.DefaultClient, server.URL, "u_bad")
	if !errors.Is(err, ErrBearerRejected) {
		t.Errorf("err = %v, want ErrBearerRejected", err)
	}
}

func TestFetchMe_GraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"nope"}]}`))
	}))
	defer server.Close()

	_, err := FetchMe(context.Background(), http.DefaultClient, server.URL, "u_test")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v, want to contain 'nope'", err)
	}
}
