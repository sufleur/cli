package userapi

import (
	"context"
	"time"
)

// PromptVersion mirrors the queryable subset of the GraphQL PromptVersion
// type that the CLI surfaces. Metadata and OutputSchema are JSON scalars so
// they decode into open maps. OutputSchema is nullable on the wire; the
// pointer distinguishes "no schema set" from "empty object". ModelConfig is a
// structured object (not a JSON scalar); it is nullable on the wire, so the
// pointer distinguishes "no model config set" from a present-but-empty one.
type PromptVersion struct {
	Version      string         `json:"version"`
	Status       string         `json:"status"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	Metadata     map[string]any `json:"metadata"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	ModelConfig  *ModelConfig   `json:"modelConfig,omitempty"`
	Readme       string         `json:"readme"`
	Files        []PromptFile   `json:"files"`
}

// ModelConfig is a version's structured provider/model/parameters, set via
// SetPromptVersionModelConfig. Provider is the LlmProvider GraphQL enum value
// (e.g. "ANTHROPIC"); Parameters is a free-form JSON object.
type ModelConfig struct {
	Provider   string         `json:"provider"`
	Model      string         `json:"model"`
	Parameters map[string]any `json:"parameters"`
}

// PromptFile is one file inside a PromptVersion. Content may be empty for
// queries that don't request it; the JSON tag preserves the field on output.
type PromptFile struct {
	Name         string `json:"name"`
	Content      string `json:"content"`
	IsEntrypoint bool   `json:"isEntrypoint"`
}

// PromptVersionsPage is the response shape of Prompt.versions.
type PromptVersionsPage struct {
	Data  []PromptVersion `json:"data"`
	Total int             `json:"total"`
}

const promptVersionFields = "version status createdAt updatedAt metadata outputSchema modelConfig { provider model parameters } readme files { name content isEntrypoint }"

// CreatePromptVersion creates a new draft version of an existing prompt by
// copying its latest published version.
func (c *Client) CreatePromptVersion(ctx context.Context, workspace, name string) (*PromptVersion, error) {
	var resp struct {
		Version *PromptVersion `json:"createPromptVersion"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreatePromptVersion($promptName: ID!) { createPromptVersion(promptName: $promptName) { " + promptVersionFields + " } }",
		Variables: map[string]any{"promptName": name},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("createPromptVersion")
	}
	return resp.Version, nil
}

// DeletePromptVersion deletes a draft version. The backend rejects deleting
// published versions and surfaces that as a GraphQL error.
func (c *Client) DeletePromptVersion(ctx context.Context, workspace, name, version string) (bool, error) {
	var resp struct {
		Deleted bool `json:"deletePromptVersion"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation DeletePromptVersion($promptName: ID!, $version: ID!) { deletePromptVersion(promptName: $promptName, version: $version) }",
		Variables: map[string]any{"promptName": name, "version": version},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return false, err
	}
	return resp.Deleted, nil
}

// GetPromptVersion resolves a single version by constraint ("1.2.3", "^1.0",
// "draft", etc.) and returns its full content including file bodies.
func (c *Client) GetPromptVersion(ctx context.Context, workspace, name, constraint string) (*PromptVersion, error) {
	var resp struct {
		Prompt struct {
			Version *PromptVersion `json:"version"`
		} `json:"prompt"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetPromptVersion($promptName: ID!, $constraint: String!) { prompt(promptName: $promptName) { version(constraint: $constraint) { " + promptVersionFields + " } } }",
		Variables: map[string]any{"promptName": name, "constraint": constraint},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompt.Version == nil {
		return nil, errMissingData("version")
	}
	return resp.Prompt.Version, nil
}

// ListPromptVersions paginates the versions of a prompt. status filters to
// DRAFT or PUBLISHED when non-empty.
func (c *Client) ListPromptVersions(ctx context.Context, workspace, name, status string, take, skip int) (*PromptVersionsPage, error) {
	vars := map[string]any{
		"promptName": name,
		"pagination": map[string]int{"take": take, "skip": skip},
	}
	if status != "" {
		vars["status"] = status
	}
	var resp struct {
		Prompt struct {
			Versions *PromptVersionsPage `json:"versions"`
		} `json:"prompt"`
	}
	err := c.Do(ctx, Request{
		Query:     "query ListPromptVersions($promptName: ID!, $pagination: PaginationArgs!, $status: PromptVersionStatus) { prompt(promptName: $promptName) { versions(pagination: $pagination, status: $status) { data { " + promptVersionFields + " } total } } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompt.Versions == nil {
		return nil, errMissingData("versions")
	}
	return resp.Prompt.Versions, nil
}

// SetPromptVersionStringMetadata sets a string-typed metadata key.
func (c *Client) SetPromptVersionStringMetadata(ctx context.Context, workspace, name, version, key, value string) (*PromptVersion, error) {
	return c.setMetadata(ctx, workspace, "promptVersionSetStringMetadata", "String!", name, version, key, value)
}

// SetPromptVersionIntegerMetadata sets an integer-typed metadata key.
func (c *Client) SetPromptVersionIntegerMetadata(ctx context.Context, workspace, name, version, key string, value int64) (*PromptVersion, error) {
	return c.setMetadata(ctx, workspace, "promptVersionSetIntegerMetadata", "Int!", name, version, key, value)
}

// SetPromptVersionFloatMetadata sets a float-typed metadata key.
func (c *Client) SetPromptVersionFloatMetadata(ctx context.Context, workspace, name, version, key string, value float64) (*PromptVersion, error) {
	return c.setMetadata(ctx, workspace, "promptVersionSetFloatMetadata", "Float!", name, version, key, value)
}

// SetPromptVersionBooleanMetadata sets a boolean-typed metadata key.
func (c *Client) SetPromptVersionBooleanMetadata(ctx context.Context, workspace, name, version, key string, value bool) (*PromptVersion, error) {
	return c.setMetadata(ctx, workspace, "promptVersionSetBooleanMetadata", "Boolean!", name, version, key, value)
}

func (c *Client) setMetadata(ctx context.Context, workspace, mutation, valueType, name, version, key string, value any) (*PromptVersion, error) {
	query := "mutation Set($promptName: ID!, $version: ID!, $metadataKey: String!, $metadataValue: " + valueType + ") { " +
		mutation + "(promptName: $promptName, version: $version, metadataKey: $metadataKey, metadataValue: $metadataValue) { " + promptVersionFields + " } }"
	var resp map[string]*PromptVersion
	err := c.Do(ctx, Request{
		Query: query,
		Variables: map[string]any{
			"promptName":    name,
			"version":       version,
			"metadataKey":   key,
			"metadataValue": value,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	v := resp[mutation]
	if v == nil {
		return nil, errMissingData(mutation)
	}
	return v, nil
}

// DeletePromptVersionMetadata clears a single metadata key on a version.
func (c *Client) DeletePromptVersionMetadata(ctx context.Context, workspace, name, version, key string) (*PromptVersion, error) {
	var resp struct {
		Version *PromptVersion `json:"promptVersionDeleteMetadata"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation DeleteMetadata($promptName: ID!, $version: ID!, $metadataKey: String!) { promptVersionDeleteMetadata(promptName: $promptName, version: $version, metadataKey: $metadataKey) { " + promptVersionFields + " } }",
		Variables: map[string]any{
			"promptName":  name,
			"version":     version,
			"metadataKey": key,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("promptVersionDeleteMetadata")
	}
	return resp.Version, nil
}

// SetPromptVersionOutputSchema replaces a version's output schema. Pass nil
// to clear it.
func (c *Client) SetPromptVersionOutputSchema(ctx context.Context, workspace, name, version string, schema map[string]any) (*PromptVersion, error) {
	var resp struct {
		Version *PromptVersion `json:"promptVersionSetOutputSchema"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation SetOutputSchema($promptName: ID!, $version: ID!, $outputSchema: JSON) { promptVersionSetOutputSchema(promptName: $promptName, version: $version, outputSchema: $outputSchema) { " + promptVersionFields + " } }",
		Variables: map[string]any{
			"promptName":   name,
			"version":      version,
			"outputSchema": schema,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("promptVersionSetOutputSchema")
	}
	return resp.Version, nil
}

// SetPromptVersionModelConfig replaces a version's structured model config
// (provider, model, parameters).
func (c *Client) SetPromptVersionModelConfig(ctx context.Context, workspace, name, version string, modelConfig ModelConfig) (*PromptVersion, error) {
	var resp struct {
		Version *PromptVersion `json:"promptVersionSetModelConfig"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation SetModelConfig($promptName: ID!, $version: ID!, $modelConfig: ModelConfigInput!) { promptVersionSetModelConfig(promptName: $promptName, version: $version, modelConfig: $modelConfig) { " + promptVersionFields + " } }",
		Variables: map[string]any{
			"promptName": name,
			"version":    version,
			"modelConfig": map[string]any{
				"provider":   modelConfig.Provider,
				"model":      modelConfig.Model,
				"parameters": modelConfig.Parameters,
			},
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("promptVersionSetModelConfig")
	}
	return resp.Version, nil
}

// SetPromptVersionReadme replaces a draft version's README. The backend
// enforces a length limit and rejects writes to published versions; both
// surface as GraphQL errors.
func (c *Client) SetPromptVersionReadme(ctx context.Context, workspace, name, version, readme string) (*PromptVersion, error) {
	var resp struct {
		Version *PromptVersion `json:"promptVersionSetReadme"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation SetReadme($promptName: ID!, $version: ID!, $readme: String!) { promptVersionSetReadme(promptName: $promptName, version: $version, readme: $readme) { " + promptVersionFields + " } }",
		Variables: map[string]any{
			"promptName": name,
			"version":    version,
			"readme":     readme,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("promptVersionSetReadme")
	}
	return resp.Version, nil
}
