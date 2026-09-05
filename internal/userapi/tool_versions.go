package userapi

import (
	"context"
	"fmt"
	"time"
)

// ToolVersion mirrors the queryable subset of the GraphQL ToolVersion type.
//
// ModelDescription is the model-facing text — what is sent as the tool's
// description on the wire — and is versioned. It is not Tool.Description, which
// is the unversioned catalog blurb.
//
// InputSchema is always an object schema (the server rejects anything else).
// OutputSchema is nullable, so the map is nil when the version has none.
type ToolVersion struct {
	Version          string         `json:"version"`
	Status           string         `json:"status"`
	ModelDescription string         `json:"modelDescription"`
	InputSchema      map[string]any `json:"inputSchema"`
	OutputSchema     map[string]any `json:"outputSchema,omitempty"`
	Metadata         map[string]any `json:"metadata"`
	Readme           string         `json:"readme"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// ToolVersionsPage is the response shape of Tool.versions.
type ToolVersionsPage struct {
	Data  []ToolVersion `json:"data"`
	Total int           `json:"total"`
}

const (
	toolVersionFields     = "version status modelDescription inputSchema outputSchema metadata readme createdAt updatedAt"
	toolVersionListFields = "version status createdAt updatedAt"
)

// GetToolVersion resolves one version by semver constraint, or the literal
// "draft". A version that does not match returns a not-found error rather than
// a nil version, so callers get an actionable message.
func (c *Client) GetToolVersion(ctx context.Context, workspace, name, constraint string) (*ToolVersion, error) {
	var resp struct {
		Tool *struct {
			Version *ToolVersion `json:"version"`
		} `json:"tool"`
	}
	err := c.Do(ctx, Request{
		Query: "query GetToolVersion($toolName: ID!, $constraint: String!) { tool(toolName: $toolName) { version(constraint: $constraint) { " +
			toolVersionFields + " } } }",
		Variables: map[string]any{"toolName": name, "constraint": constraint},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Tool == nil {
		return nil, errMissingData("tool")
	}
	if resp.Tool.Version == nil {
		return nil, fmt.Errorf("no version of tool %q matches %q", name, constraint)
	}
	return resp.Tool.Version, nil
}

// ListToolVersions returns a page of a tool's versions, optionally filtered by
// status ("DRAFT" or "PUBLISHED").
func (c *Client) ListToolVersions(ctx context.Context, workspace, name, status string, take, skip int) (*ToolVersionsPage, error) {
	vars := map[string]any{
		"toolName":   name,
		"pagination": map[string]int{"take": take, "skip": skip},
	}
	statusArg := ""
	if status != "" {
		vars["status"] = status
		statusArg = ", status: $status"
	}
	decl := "$toolName: ID!, $pagination: PaginationArgs!"
	if status != "" {
		decl += ", $status: ToolVersionStatus"
	}

	var resp struct {
		Tool *struct {
			Versions *ToolVersionsPage `json:"versions"`
		} `json:"tool"`
	}
	err := c.Do(ctx, Request{
		Query: "query ListToolVersions(" + decl + ") { tool(toolName: $toolName) { versions(pagination: $pagination" +
			statusArg + ") { data { " + toolVersionListFields + " } total } } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Tool == nil || resp.Tool.Versions == nil {
		return nil, errMissingData("tool.versions")
	}
	return resp.Tool.Versions, nil
}

// CreateToolVersionDraft opens a new draft, carrying the latest published
// version's contract forward. Rejected while a draft is already open.
func (c *Client) CreateToolVersionDraft(ctx context.Context, workspace, name string) (*ToolVersion, error) {
	var resp struct {
		Version *ToolVersion `json:"createToolVersionDraft"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreateToolVersionDraft($toolName: ID!) { createToolVersionDraft(toolName: $toolName) { " + toolVersionFields + " } }",
		Variables: map[string]any{"toolName": name},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("createToolVersionDraft")
	}
	return resp.Version, nil
}

// DeleteToolVersion deletes a draft version. Published versions are immutable
// and the server rejects deleting them.
func (c *Client) DeleteToolVersion(ctx context.Context, workspace, name, version string) error {
	var resp struct {
		Deleted bool `json:"deleteToolVersion"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation DeleteToolVersion($toolName: ID!, $version: ID!) { deleteToolVersion(toolName: $toolName, version: $version) }",
		Variables: map[string]any{"toolName": name, "version": version},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return err
	}
	if !resp.Deleted {
		return fmt.Errorf("tool version %q was not deleted", version)
	}
	return nil
}

// updateToolVersion is the single write path for a draft's contract. Every
// setter funnels through it so the partial-update semantics — only the keys
// present in input are touched — live in one place.
func (c *Client) updateToolVersion(ctx context.Context, workspace, name, version string, input map[string]any) (*ToolVersion, error) {
	var resp struct {
		Version *ToolVersion `json:"updateToolVersion"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdateToolVersion($toolName: ID!, $version: ID!, $input: UpdateToolVersionInput!) { updateToolVersion(toolName: $toolName, version: $version, input: $input) { " +
			toolVersionFields + " } }",
		Variables: map[string]any{"toolName": name, "version": version, "input": input},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("updateToolVersion")
	}
	return resp.Version, nil
}

// SetToolVersionModelDescription replaces the model-facing description.
func (c *Client) SetToolVersionModelDescription(ctx context.Context, workspace, name, version, description string) (*ToolVersion, error) {
	return c.updateToolVersion(ctx, workspace, name, version, map[string]any{"modelDescription": description})
}

// SetToolVersionInputSchema replaces the argument schema.
func (c *Client) SetToolVersionInputSchema(ctx context.Context, workspace, name, version string, schema map[string]any) (*ToolVersion, error) {
	return c.updateToolVersion(ctx, workspace, name, version, map[string]any{"inputSchema": schema})
}

// SetToolVersionOutputSchema replaces the result schema. A nil schema clears it.
func (c *Client) SetToolVersionOutputSchema(ctx context.Context, workspace, name, version string, schema map[string]any) (*ToolVersion, error) {
	return c.updateToolVersion(ctx, workspace, name, version, map[string]any{"outputSchema": schema})
}

// SetToolVersionReadme replaces the README.
func (c *Client) SetToolVersionReadme(ctx context.Context, workspace, name, version, readme string) (*ToolVersion, error) {
	return c.updateToolVersion(ctx, workspace, name, version, map[string]any{"readme": readme})
}

// SetToolVersionMetadata replaces the whole metadata object.
//
// Unlike a prompt version's {type, value} metadata, a tool version's is a plain
// JSON blob with no per-key mutation on the server — so a single-key change is
// a client-side read-modify-write, and concurrent edits can lose one another.
func (c *Client) SetToolVersionMetadata(ctx context.Context, workspace, name, version string, metadata map[string]any) (*ToolVersion, error) {
	return c.updateToolVersion(ctx, workspace, name, version, map[string]any{"metadata": metadata})
}
