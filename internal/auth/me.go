package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Me is the subset of the user record we currently surface in the CLI.
type Me struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type meResponse struct {
	Data struct {
		Me *Me `json:"me"`
	} `json:"data"`
	Errors []graphqlError `json:"errors"`
}

// FetchMe runs the `me` GraphQL query using bearerKey for authentication.
// Returns ErrBearerRejected if the server refuses the bearer.
func FetchMe(ctx context.Context, hc *http.Client, baseURL, bearerKey string) (*Me, error) {
	body, err := json.Marshal(graphqlRequest{
		Query: "query Me { me { id email } }",
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling me request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(baseURL, "/graphql"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerKey)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("me request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return nil, ErrBearerRejected
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("me request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var out meResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parsing me response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("me query failed: %s", out.Errors[0].Message)
	}
	if out.Data.Me == nil {
		return nil, fmt.Errorf("me query returned no data")
	}
	return out.Data.Me, nil
}
