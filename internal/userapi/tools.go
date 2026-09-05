package userapi

import (
	"context"
	"time"
)

// Tool mirrors the queryable subset of the GraphQL Tool type the CLI surfaces.
//
// Description is the catalog blurb: unversioned, used for listing and search,
// and never sent to the model. The model-facing text is
// ToolVersion.ModelDescription, which is versioned. Conflating the two is the
// easiest mistake to make with this entity.
type Tool struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Visibility  string            `json:"visibility"`
	Tags        []string          `json:"tags"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Versions    *ToolVersionsPage `json:"versions,omitempty"`
	// DependentCount is the number of published prompt versions pinning any
	// version of this tool, across every workspace. Members-only on the server.
	DependentCount *int `json:"dependentCount,omitempty"`
}

// ToolsPage is the response shape of the `tools` query.
type ToolsPage struct {
	Data  []Tool `json:"data"`
	Total int    `json:"total"`
}

const toolFields = "id name description visibility tags createdAt updatedAt"

// GetTool fetches a tool by its bare name, with a page of its versions and the
// count of published prompt versions depending on it.
func (c *Client) GetTool(ctx context.Context, workspace, name string) (*Tool, error) {
	var resp struct {
		Tool *Tool `json:"tool"`
	}
	err := c.Do(ctx, Request{
		Query: "query GetTool($toolName: ID!, $pagination: PaginationArgs!) { tool(toolName: $toolName) { " +
			toolFields + " dependentCount versions(pagination: $pagination) { data { " +
			toolVersionListFields + " } total } } }",
		Variables: map[string]any{
			"toolName":   name,
			"pagination": map[string]int{"take": 100, "skip": 0},
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Tool == nil {
		return nil, errMissingData("tool")
	}
	return resp.Tool, nil
}

// ListTools paginates through the tools in a workspace. search is optional.
func (c *Client) ListTools(ctx context.Context, workspace, search string, take, skip int) (*ToolsPage, error) {
	vars := map[string]any{"pagination": map[string]int{"take": take, "skip": skip}}
	if search != "" {
		vars["search"] = search
	}
	var resp struct {
		Tools *ToolsPage `json:"tools"`
	}
	err := c.Do(ctx, Request{
		Query: "query ListTools($pagination: PaginationArgs!, $search: String) { tools(pagination: $pagination, search: $search) { data { " +
			toolFields + " } total } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Tools == nil {
		return nil, errMissingData("tools")
	}
	return resp.Tools, nil
}

// CreateTool creates a tool and its initial draft version. Tools are always
// created private; making one public is web-app-only.
func (c *Client) CreateTool(ctx context.Context, workspace, name, description string) (*Tool, error) {
	vars := map[string]any{"name": name}
	if description != "" {
		vars["description"] = description
	}
	var resp struct {
		Tool *Tool `json:"createTool"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreateTool($name: String!, $description: String) { createTool(name: $name, description: $description) { " + toolFields + " } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Tool == nil {
		return nil, errMissingData("createTool")
	}
	return resp.Tool, nil
}

// UpdateTool replaces the catalog description on an existing tool. This is not
// the model-facing text — see SetToolVersionModelDescription for that.
func (c *Client) UpdateTool(ctx context.Context, workspace, name, description string) (*Tool, error) {
	var resp struct {
		Tool *Tool `json:"updateTool"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdateTool($toolName: ID!, $args: UpdateToolArgs!) { updateTool(toolName: $toolName, args: $args) { " + toolFields + " } }",
		Variables: map[string]any{
			"toolName": name,
			"args":     map[string]any{"description": description},
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Tool == nil {
		return nil, errMissingData("updateTool")
	}
	return resp.Tool, nil
}
