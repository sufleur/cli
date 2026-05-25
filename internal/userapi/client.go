// Package userapi is the HTTP/GraphQL client used by commands that authenticate
// with a user API key (the `u_*` prefix issued by `sufleur login`). It posts to
// {base}/graphql with the bearer token, optionally adds an X-Workspace header
// per request, and decodes the standard GraphQL envelope.
//
// Commands that use a workspace API key continue to use internal/fetcher.
package userapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ErrBearerRejected is returned when the GraphQL endpoint refuses the bearer
// token (401/403/404). Typically means the stored key was revoked or is no
// longer recognised; callers should usually treat this as "re-run `sufleur login`".
var ErrBearerRejected = errors.New("bearer token rejected")

// Client posts authenticated GraphQL requests against a Sufleur API base.
type Client struct {
	base   string
	apiKey string
	http   *http.Client
}

// New constructs a Client. When verbose is true, request and response bodies
// are logged to stderr.
func New(apiBase, apiKey string, verbose bool) *Client {
	hc := &http.Client{}
	if verbose {
		hc.Transport = &debugTransport{wrapped: http.DefaultTransport}
	}
	return &Client{
		base:   strings.TrimRight(apiBase, "/"),
		apiKey: apiKey,
		http:   hc,
	}
}

// Request is one GraphQL operation. Variables and Workspace are optional.
type Request struct {
	Query     string
	Variables map[string]any
	// Workspace sets the X-Workspace header if non-empty. Omit for
	// user-scoped operations (me, userApiKeys, revokeUserApiKey).
	Workspace string
}

// Do executes req and decodes the response's `data` field into result. If
// result is nil, the data field is discarded. Returns ErrBearerRejected on
// 401/403/404, and a wrapped error formed from the first GraphQL error if the
// response contains `errors[]`.
func (c *Client) Do(ctx context.Context, req Request, result any) error {
	body, err := json.Marshal(graphqlRequest{Query: req.Query, Variables: req.Variables})
	if err != nil {
		return fmt.Errorf("marshaling graphql request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("X-Client", "cli")
	if req.Workspace != "" {
		httpReq.Header.Set("X-Workspace", req.Workspace)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return ErrBearerRejected
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql request returned %d: %s", resp.StatusCode, string(respBody))
	}

	var env graphqlEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return fmt.Errorf("parsing graphql response: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", env.Errors[0].Message)
	}
	if result == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, result); err != nil {
		return fmt.Errorf("decoding graphql data: %w", err)
	}
	return nil
}

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlError struct {
	Message string `json:"message"`
}

type graphqlEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors"`
}

func errMissingData(field string) error {
	return fmt.Errorf("graphql response missing %q field", field)
}

// debugTransport logs request and response bodies to stderr.
type debugTransport struct {
	wrapped http.RoundTripper
}

func (d *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		fmt.Fprintf(os.Stderr, "[verbose] → %s %s\n[verbose] Request body: %s\n", req.Method, req.URL, body)
	}
	resp, err := d.wrapped.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	fmt.Fprintf(os.Stderr, "[verbose] ← %d %s\n", resp.StatusCode, body)
	return resp, nil
}
