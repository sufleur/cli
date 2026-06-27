package userapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListProviderCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Workspace") != "acme" {
			t.Errorf("X-Workspace = %q, want acme", r.Header.Get("X-Workspace"))
		}
		_, _ = w.Write([]byte(`{"data":{"workspace":{"providerCredentials":[{"id":"c1","provider":"ANTHROPIC","name":"prod","lastFour":"1234","createdAt":"2024-03-12T10:23:45Z"}]}}}`))
	}))
	defer server.Close()

	creds, err := New(server.URL, "u_test", false).ListProviderCredentials(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ListProviderCredentials: %v", err)
	}
	if len(creds) != 1 || creds[0].Provider != "ANTHROPIC" || creds[0].LastFour != "1234" {
		t.Errorf("got %+v", creds)
	}
}

func TestClient_ListProviderCredentials_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"workspace":{"providerCredentials":[]}}}`))
	}))
	defer server.Close()

	creds, err := New(server.URL, "u_test", false).ListProviderCredentials(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ListProviderCredentials: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("expected empty, got %+v", creds)
	}
}

func TestClient_AvailableModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if req.Variables["provider"] != "OPENAI" {
			t.Errorf("provider = %v, want OPENAI (uppercase)", req.Variables["provider"])
		}
		_, _ = w.Write([]byte(`{"data":{"availableModels":[{"id":"gpt-4o","provider":"OPENAI","displayName":"GPT-4o","contextWindow":128000,"maxOutputTokens":16384,"source":"CATALOG"}]}}`))
	}))
	defer server.Close()

	models, err := New(server.URL, "u_test", false).AvailableModels(context.Background(), "acme", "OPENAI")
	if err != nil {
		t.Fatalf("AvailableModels: %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-4o" || models[0].ContextWindow != 128000 {
		t.Errorf("got %+v", models)
	}
}

func TestNormalizeProvider(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"anthropic", "ANTHROPIC", true},
		{"OpenAI", "OPENAI", true},
		{"  google  ", "GOOGLE", true},
		{"bogus", "BOGUS", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeProvider(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NormalizeProvider(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
