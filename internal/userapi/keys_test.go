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

func TestClient_RevokeUserAPIKey_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "revokeUserApiKey") {
			t.Errorf("query missing revokeUserApiKey: %q", req.Query)
		}
		if req.Variables["id"] != "key-1" {
			t.Errorf("id var = %v", req.Variables["id"])
		}
		_, _ = w.Write([]byte(`{"data":{"revokeUserApiKey":true}}`))
	}))
	defer server.Close()

	ok, err := New(server.URL, "u_test", false).RevokeUserAPIKey(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if !ok {
		t.Error("ok = false, want true")
	}
}

func TestClient_RevokeUserAPIKey_FalseReturn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"revokeUserApiKey":false}}`))
	}))
	defer server.Close()

	ok, err := New(server.URL, "u_test", false).RevokeUserAPIKey(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if ok {
		t.Error("ok = true, want false")
	}
}

func TestClient_RevokeUserAPIKey_BearerRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(server.URL, "u_bad", false).RevokeUserAPIKey(context.Background(), "key-1")
	if !errors.Is(err, ErrBearerRejected) {
		t.Errorf("err = %v, want ErrBearerRejected", err)
	}
}
