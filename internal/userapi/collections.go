package userapi

import (
	"context"
	"time"
)

// Collection mirrors the queryable subset of the GraphQL PromptCollection type
// that the CLI surfaces. Timestamps decode from the backend's DateTime scalar
// (RFC 3339). The "+" collection marker is a CLI-only concern: every name here
// is the bare backend name without it.
type Collection struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Readme      string    `json:"readme"`
	Visibility  string    `json:"visibility"`
	PromptCount int       `json:"promptCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const collectionFields = "name description readme visibility promptCount createdAt updatedAt"

// GetCollection fetches a single collection by its bare name. The workspace is
// sent via the X-Workspace header.
func (c *Client) GetCollection(ctx context.Context, workspace, name string) (*Collection, error) {
	var resp struct {
		Collection *Collection `json:"promptCollection"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetCollection($name: ID!) { promptCollection(name: $name) { " + collectionFields + " } }",
		Variables: map[string]any{"name": name},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Collection == nil {
		return nil, errMissingData("promptCollection")
	}
	return resp.Collection, nil
}

// ListCollectionPrompts returns the bare names of the prompts in a collection,
// ordered as the backend returns them (most recently updated first).
func (c *Client) ListCollectionPrompts(ctx context.Context, workspace, name string) ([]string, error) {
	var resp struct {
		Collection *struct {
			Prompts []struct {
				Name string `json:"name"`
			} `json:"prompts"`
		} `json:"promptCollection"`
	}
	err := c.Do(ctx, Request{
		Query:     "query ListCollectionPrompts($name: ID!) { promptCollection(name: $name) { prompts { name } } }",
		Variables: map[string]any{"name": name},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Collection == nil {
		return nil, errMissingData("promptCollection")
	}
	names := make([]string, 0, len(resp.Collection.Prompts))
	for _, p := range resp.Collection.Prompts {
		names = append(names, p.Name)
	}
	return names, nil
}

// CreateCollection creates a new collection. description is optional; pass an
// empty string to omit it (the field is nullable on the wire). Visibility is
// not exposed by the CLI — the backend defaults new collections to PRIVATE.
func (c *Client) CreateCollection(ctx context.Context, workspace, name, description string) (*Collection, error) {
	vars := map[string]any{"name": name}
	if description != "" {
		vars["description"] = description
	}
	var resp struct {
		Collection *Collection `json:"createPromptCollection"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreateCollection($name: String!, $description: String) { createPromptCollection(name: $name, description: $description) { " + collectionFields + " } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Collection == nil {
		return nil, errMissingData("createPromptCollection")
	}
	return resp.Collection, nil
}

// UpdateCollectionDescription replaces the description on an existing
// collection. The readme is preserved (the backend keeps fields omitted from
// args). Edits are immediately live — collections have no draft workflow.
func (c *Client) UpdateCollectionDescription(ctx context.Context, workspace, name, description string) (*Collection, error) {
	return c.updateCollection(ctx, workspace, name, map[string]any{"description": description})
}

// UpdateCollectionReadme replaces the readme on an existing collection. The
// description is preserved.
func (c *Client) UpdateCollectionReadme(ctx context.Context, workspace, name, readme string) (*Collection, error) {
	return c.updateCollection(ctx, workspace, name, map[string]any{"readme": readme})
}

func (c *Client) updateCollection(ctx context.Context, workspace, name string, args map[string]any) (*Collection, error) {
	var resp struct {
		Collection *Collection `json:"updatePromptCollection"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdateCollection($name: ID!, $args: UpdatePromptCollectionArgs!) { updatePromptCollection(name: $name, args: $args) { " + collectionFields + " } }",
		Variables: map[string]any{
			"name": name,
			"args": args,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Collection == nil {
		return nil, errMissingData("updatePromptCollection")
	}
	return resp.Collection, nil
}

// SetPromptCollection links a prompt to a collection. Both names are bare
// (workspace comes from the header) and must live in the same workspace. The
// CLI never passes a null collection — detaching is intentionally unsupported.
func (c *Client) SetPromptCollection(ctx context.Context, workspace, promptName, collectionName string) error {
	return c.Do(ctx, Request{
		Query: "mutation SetPromptCollection($promptName: ID!, $collectionName: ID) { setPromptCollection(promptName: $promptName, collectionName: $collectionName) { name } }",
		Variables: map[string]any{
			"promptName":     promptName,
			"collectionName": collectionName,
		},
		Workspace: workspace,
	}, nil)
}

// GetPromptCurrentCollection returns the bare name of the collection a prompt
// currently belongs to, or "" if it is not in any collection. Used to guard
// `collection link` against silently moving a prompt out of another collection.
func (c *Client) GetPromptCurrentCollection(ctx context.Context, workspace, promptName string) (string, error) {
	var resp struct {
		Prompt *struct {
			Collection *struct {
				Name string `json:"name"`
			} `json:"collection"`
		} `json:"prompt"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetPromptCollection($promptName: ID!) { prompt(promptName: $promptName) { collection { name } } }",
		Variables: map[string]any{"promptName": promptName},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return "", err
	}
	if resp.Prompt == nil {
		return "", errMissingData("prompt")
	}
	if resp.Prompt.Collection == nil {
		return "", nil
	}
	return resp.Prompt.Collection.Name, nil
}
