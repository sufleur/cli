package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrBearerRejected is returned when the GraphQL endpoint refuses the bearer
// token (typically because the key has already been revoked server-side).
// Callers should treat this as "local credentials are stale, delete them".
var ErrBearerRejected = errors.New("bearer token rejected")

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlError struct {
	Message string `json:"message"`
}

type revokeResponse struct {
	Data struct {
		RevokeUserAPIKey bool `json:"revokeUserApiKey"`
	} `json:"data"`
	Errors []graphqlError `json:"errors"`
}

// RevokeUserAPIKey calls the revokeUserApiKey mutation against {baseURL}/graphql
// using bearerKey for authentication. Returns true if the backend reports the
// key was revoked; false if the backend reports it was already gone. Returns
// ErrBearerRejected if the bearer itself is no longer valid.
func RevokeUserAPIKey(ctx context.Context, hc *http.Client, baseURL, bearerKey, keyID string) (bool, error) {
	body, err := json.Marshal(graphqlRequest{
		Query:     "mutation RevokeUserApiKey($id: ID!) { revokeUserApiKey(id: $id) }",
		Variables: map[string]any{"id": keyID},
	})
	if err != nil {
		return false, fmt.Errorf("marshaling revoke request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(baseURL, "/graphql"), bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerKey)

	resp, err := hc.Do(req)
	if err != nil {
		return false, fmt.Errorf("revoke request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return false, ErrBearerRejected
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("revoke request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var out revokeResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return false, fmt.Errorf("parsing revoke response: %w", err)
	}
	if len(out.Errors) > 0 {
		// GraphQL-level error — most likely "key not found" or "not authorized".
		// Either way the local credentials are stale; surface the message but
		// let the caller decide how to proceed.
		return false, fmt.Errorf("revoke failed: %s", out.Errors[0].Message)
	}
	return out.Data.RevokeUserAPIKey, nil
}
