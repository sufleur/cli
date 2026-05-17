// Package auth implements the client side of the device-code OAuth flow
// (RFC 8628) used by `sufleur login`, plus the GraphQL operations that the
// login / logout commands need (currently just revokeUserApiKey).
//
// The package exposes the HTTP primitives and typed errors so the CLI layer
// can own the interactive polling loop, output formatting, and verbose logging.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DeviceCode is the response from /oauth/device/code.
type DeviceCode struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expiresIn"`
}

// Token is the successful response from /oauth/device/token.
type Token struct {
	APIKey string `json:"apiKey"`
	UserID string `json:"userId"`
	KeyID  string `json:"keyId"`
}

// Typed errors for the polling state machine. The first two are non-terminal:
// the caller should wait and poll again. The rest are terminal.
var (
	ErrAuthorizationPending = errors.New("authorization pending")
	ErrSlowDown             = errors.New("slow down")
	ErrAccessDenied         = errors.New("access denied")
	ErrExpiredToken         = errors.New("expired token")
)

type oauthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// RequestDeviceCode initiates a device-code flow. deviceLabel is optional
// (sent as the user-visible label for the issued key, max 255 chars).
func RequestDeviceCode(ctx context.Context, hc *http.Client, baseURL, deviceLabel string) (*DeviceCode, error) {
	body, err := json.Marshal(map[string]string{"deviceLabel": deviceLabel})
	if err != nil {
		return nil, fmt.Errorf("marshaling device-code request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(baseURL, "/oauth/device/code"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device-code request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var dc DeviceCode
	if err := json.Unmarshal(respBody, &dc); err != nil {
		return nil, fmt.Errorf("parsing device-code response: %w", err)
	}
	return &dc, nil
}

// PollToken makes a single /oauth/device/token request. On 200, returns the
// minted token. On 400 with a known OAuth error code, returns the matching
// typed error (callers compare with errors.Is). Other responses return a raw
// error describing the wire response.
func PollToken(ctx context.Context, hc *http.Client, baseURL, deviceCode string) (*Token, error) {
	body, err := json.Marshal(map[string]string{"deviceCode": deviceCode})
	if err != nil {
		return nil, fmt.Errorf("marshaling token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(baseURL, "/oauth/device/token"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polling token: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var t Token
		if err := json.Unmarshal(respBody, &t); err != nil {
			return nil, fmt.Errorf("parsing token response: %w", err)
		}
		return &t, nil
	}

	if resp.StatusCode == http.StatusBadRequest {
		var oe oauthError
		if err := json.Unmarshal(respBody, &oe); err == nil && oe.Error != "" {
			switch oe.Error {
			case "authorization_pending":
				return nil, ErrAuthorizationPending
			case "slow_down":
				return nil, ErrSlowDown
			case "access_denied":
				return nil, ErrAccessDenied
			case "expired_token":
				return nil, ErrExpiredToken
			default:
				return nil, fmt.Errorf("token request failed: %s (%s)", oe.Error, oe.ErrorDescription)
			}
		}
	}

	return nil, fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(respBody))
}

func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}
