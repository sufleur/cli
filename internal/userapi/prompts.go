package userapi

import (
	"context"
	"time"
)

// Prompt mirrors the queryable subset of the GraphQL Prompt type that the
// CLI currently surfaces. Timestamps decode from the backend's DateTime
// scalar (RFC 3339).
type Prompt struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// PromptsPage is the response shape of the `prompts` query.
type PromptsPage struct {
	Data  []Prompt `json:"data"`
	Total int      `json:"total"`
}

const promptFields = "name description visibility createdAt updatedAt"

// GetPrompt fetches a single prompt by its bare name. The workspace is sent
// via the X-Workspace header.
func (c *Client) GetPrompt(ctx context.Context, workspace, name string) (*Prompt, error) {
	var resp struct {
		Prompt *Prompt `json:"prompt"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetPrompt($promptName: ID!) { prompt(promptName: $promptName) { " + promptFields + " } }",
		Variables: map[string]any{"promptName": name},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompt == nil {
		return nil, errMissingData("prompt")
	}
	return resp.Prompt, nil
}

// ListPrompts paginates through prompts in the workspace. take and skip map
// directly to PaginationArgs. search is optional.
func (c *Client) ListPrompts(ctx context.Context, workspace, search string, take, skip int) (*PromptsPage, error) {
	vars := map[string]any{
		"pagination": map[string]int{"take": take, "skip": skip},
	}
	if search != "" {
		vars["search"] = search
	}
	var resp struct {
		Prompts *PromptsPage `json:"prompts"`
	}
	err := c.Do(ctx, Request{
		Query:     "query ListPrompts($pagination: PaginationArgs!, $search: String) { prompts(pagination: $pagination, search: $search) { data { " + promptFields + " } total } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompts == nil {
		return nil, errMissingData("prompts")
	}
	return resp.Prompts, nil
}

// CreatePrompt creates a new prompt. description is optional; pass an empty
// string to omit it (the field is nullable on the wire). Visibility is not
// exposed by the CLI per design.
func (c *Client) CreatePrompt(ctx context.Context, workspace, name, description string) (*Prompt, error) {
	vars := map[string]any{"name": name}
	if description != "" {
		vars["description"] = description
	}
	var resp struct {
		Prompt *Prompt `json:"createPrompt"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreatePrompt($name: String!, $description: String) { createPrompt(name: $name, description: $description) { " + promptFields + " } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompt == nil {
		return nil, errMissingData("createPrompt")
	}
	return resp.Prompt, nil
}

// UpdatePromptDescription replaces the description on an existing prompt.
// Visibility updates use a separate (intentionally unexposed) mutation.
func (c *Client) UpdatePromptDescription(ctx context.Context, workspace, name, description string) (*Prompt, error) {
	var resp struct {
		Prompt *Prompt `json:"updatePrompt"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdatePrompt($promptName: ID!, $args: UpdatePromptArgs!) { updatePrompt(promptName: $promptName, args: $args) { " + promptFields + " } }",
		Variables: map[string]any{
			"promptName": name,
			"args":       map[string]any{"description": description},
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompt == nil {
		return nil, errMissingData("updatePrompt")
	}
	return resp.Prompt, nil
}
