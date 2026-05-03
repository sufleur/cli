package fetcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	graphql "github.com/hasura/go-graphql-client"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

// debugTransport wraps an http.RoundTripper and logs request/response bodies.
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

type client struct {
	gql *graphql.Client
}

// NewClient creates a new GraphQL client for the Sufleur API.
func NewClient(endpoint, apiKey, workspace string, verbose bool) Client {
	httpClient := &http.Client{}
	if verbose {
		httpClient.Transport = &debugTransport{wrapped: http.DefaultTransport}
	}
	gql := graphql.NewClient(endpoint, httpClient).
		WithRequestModifier(func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			if workspace != "" {
				req.Header.Set("X-Workspace", workspace)
			}
		})
	return &client{gql: gql}
}

// ValidatePrompts checks that all given prompt names exist in the API.
func (c *client) ValidatePrompts(ctx context.Context, promptNames []string) error {
	if len(promptNames) == 0 {
		return nil
	}

	aliasToName := make(map[string]string, len(promptNames))
	var fields []string
	for _, name := range promptNames {
		alias := sanitizeAlias(name)
		aliasToName[alias] = name
		fields = append(fields, fmt.Sprintf(`%s: prompt(promptName: "%s") { name }`,
			alias, strings.ReplaceAll(name, `"`, `\"`)))
	}

	query := fmt.Sprintf("query ValidatePrompts { %s }", strings.Join(fields, " "))
	_, err := c.gql.ExecRaw(ctx, query, nil)
	if err == nil {
		return nil
	}

	var gqlErrors graphql.Errors
	if !errors.As(err, &gqlErrors) {
		return err
	}

	var missing []string
	for _, gqlErr := range gqlErrors {
		if len(gqlErr.Path) == 0 {
			continue
		}
		alias, ok := gqlErr.Path[0].(string)
		if !ok {
			continue
		}
		if name, exists := aliasToName[alias]; exists {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return &ValidationError{MissingPrompts: missing}
	}

	return err
}

// FetchPromptVersion retrieves a specific prompt version from the API.
func (c *client) FetchPromptVersion(ctx context.Context, promptName, constraint string, status *PromptVersionStatus) (*generator.PromptData, error) {
	var q fetchPromptVersionQuery
	variables := map[string]any{
		"promptName": GraphQLID(promptName),
		"constraint": constraint,
		"status":     status,
	}

	if err := c.gql.Query(ctx, &q, variables); err != nil {
		return nil, fmt.Errorf("fetching prompt version: %w", err)
	}

	if q.Prompt.Version == nil {
		return nil, fmt.Errorf("no version of %q matches constraint %q", promptName, constraint)
	}

	v := q.Prompt.Version
	files := make([]generator.PromptFile, len(v.Files))
	for i, f := range v.Files {
		warnings := make([]generator.SchemaWarning, len(f.SchemaWarnings))
		for j, w := range f.SchemaWarnings {
			warnings[j] = generator.SchemaWarning{Path: w.Path, Message: w.Message}
		}
		files[i] = generator.PromptFile{
			Name:           f.Name,
			Content:        f.Content,
			IsEntrypoint:   f.IsEntrypoint,
			InputSchema:    f.InputSchema,
			SchemaWarnings: warnings,
		}
	}

	return &generator.PromptData{
		Name:         promptName,
		Version:      v.Version,
		Description:  q.Prompt.Description,
		Status:       v.Status,
		Metadata:     v.Metadata,
		OutputSchema: v.OutputSchema,
		Files:        files,
	}, nil
}

// sanitizeAlias converts a prompt name to a valid GraphQL alias.
func sanitizeAlias(name string) string {
	var b strings.Builder
	b.WriteString("prompt_")
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
