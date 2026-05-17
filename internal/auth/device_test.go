package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRequestDeviceCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/device/code" {
			t.Errorf("path = %q, want /oauth/device/code", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("body unmarshal: %v", err)
		}
		if req["deviceLabel"] != "test-label" {
			t.Errorf("deviceLabel = %q, want test-label", req["deviceLabel"])
		}

		_, _ = w.Write([]byte(`{
			"deviceCode": "abc123",
			"userCode": "EG74YR88",
			"verificationUri": "https://sufleur.com/oauth/device",
			"interval": 5,
			"expiresIn": 600
		}`))
	}))
	defer server.Close()

	got, err := RequestDeviceCode(context.Background(), http.DefaultClient, server.URL, "test-label")
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}
	want := &DeviceCode{
		DeviceCode:      "abc123",
		UserCode:        "EG74YR88",
		VerificationURI: "https://sufleur.com/oauth/device",
		Interval:        5,
		ExpiresIn:       600,
	}
	if *got != *want {
		t.Errorf("got %+v, want %+v", *got, *want)
	}
}

func TestRequestDeviceCode_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`oh no`))
	}))
	defer server.Close()

	_, err := RequestDeviceCode(context.Background(), http.DefaultClient, server.URL, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q should mention 500", err.Error())
	}
}

func TestPollToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		_ = json.Unmarshal(body, &req)
		if req["deviceCode"] != "dc1" {
			t.Errorf("deviceCode = %q, want dc1", req["deviceCode"])
		}
		_, _ = w.Write([]byte(`{"apiKey":"u_xyz","userId":"u1","keyId":"k1"}`))
	}))
	defer server.Close()

	got, err := PollToken(context.Background(), http.DefaultClient, server.URL, "dc1")
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	want := &Token{APIKey: "u_xyz", UserID: "u1", KeyID: "k1"}
	if *got != *want {
		t.Errorf("got %+v, want %+v", *got, *want)
	}
}

func TestPollToken_OAuthErrors(t *testing.T) {
	cases := []struct {
		name     string
		oauthErr string
		wantErr  error
	}{
		{"pending", "authorization_pending", ErrAuthorizationPending},
		{"slow_down", "slow_down", ErrSlowDown},
		{"denied", "access_denied", ErrAccessDenied},
		{"expired", "expired_token", ErrExpiredToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"` + tc.oauthErr + `","error_description":null}`))
			}))
			defer server.Close()

			_, err := PollToken(context.Background(), http.DefaultClient, server.URL, "dc")
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestPollToken_UnknownOAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"weird_thing","error_description":"please don't"}`))
	}))
	defer server.Close()

	_, err := PollToken(context.Background(), http.DefaultClient, server.URL, "dc")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "weird_thing") {
		t.Errorf("error %q should mention the unknown code", err.Error())
	}
}

func TestPollToken_PendingThenSuccess(t *testing.T) {
	// Simulates the real flow: poll once → pending; poll again → 200.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"apiKey":"u_xyz","userId":"u1","keyId":"k1"}`))
	}))
	defer server.Close()

	if _, err := PollToken(context.Background(), http.DefaultClient, server.URL, "dc"); !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("first poll: err = %v, want ErrAuthorizationPending", err)
	}
	tok, err := PollToken(context.Background(), http.DefaultClient, server.URL, "dc")
	if err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if tok.APIKey != "u_xyz" {
		t.Errorf("APIKey = %q", tok.APIKey)
	}
}

func TestJoinURL(t *testing.T) {
	cases := []struct{ base, path, want string }{
		{"https://api.example.com", "/oauth/device/code", "https://api.example.com/oauth/device/code"},
		{"https://api.example.com/", "/oauth/device/code", "https://api.example.com/oauth/device/code"},
		{"http://localhost:3001", "/graphql", "http://localhost:3001/graphql"},
	}
	for _, tc := range cases {
		if got := joinURL(tc.base, tc.path); got != tc.want {
			t.Errorf("joinURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}
