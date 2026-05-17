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

func TestRevokeUserAPIKey_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer u_test" {
			t.Errorf("Authorization = %q, want Bearer u_test", got)
		}
		if r.URL.Path != "/graphql" {
			t.Errorf("path = %q, want /graphql", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !strings.Contains(req.Query, "revokeUserApiKey") {
			t.Errorf("query missing revokeUserApiKey: %q", req.Query)
		}
		if req.Variables["id"] != "key-id-1" {
			t.Errorf("id var = %v, want key-id-1", req.Variables["id"])
		}

		_, _ = w.Write([]byte(`{"data":{"revokeUserApiKey":true}}`))
	}))
	defer server.Close()

	revoked, err := RevokeUserAPIKey(context.Background(), http.DefaultClient, server.URL, "u_test", "key-id-1")
	if err != nil {
		t.Fatalf("RevokeUserAPIKey: %v", err)
	}
	if !revoked {
		t.Error("revoked = false, want true")
	}
}

func TestRevokeUserAPIKey_BearerRejected(t *testing.T) {
	cases := []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}
	for _, code := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"statusCode":` + http.StatusText(code) + `,"message":"User API key not found or has been revoked"}`))
		}))
		_, err := RevokeUserAPIKey(context.Background(), http.DefaultClient, server.URL, "u_bad", "key-id-1")
		server.Close()
		if !errors.Is(err, ErrBearerRejected) {
			t.Errorf("status %d: err = %v, want ErrBearerRejected", code, err)
		}
	}
}

func TestRevokeUserAPIKey_GraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"not found"}]}`))
	}))
	defer server.Close()

	_, err := RevokeUserAPIKey(context.Background(), http.DefaultClient, server.URL, "u_test", "key-id-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q should mention 'not found'", err.Error())
	}
}

func TestRevokeUserAPIKey_FalseReturn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"revokeUserApiKey":false}}`))
	}))
	defer server.Close()

	revoked, err := RevokeUserAPIKey(context.Background(), http.DefaultClient, server.URL, "u_test", "key-id-1")
	if err != nil {
		t.Fatalf("RevokeUserAPIKey: %v", err)
	}
	if revoked {
		t.Error("revoked = true, want false")
	}
}
