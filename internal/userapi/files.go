package userapi

import "context"

// CreatePromptFile adds a new file to a draft version. isEntrypoint is
// optional; pass false (or rely on the zero value) to leave the file as a
// non-entrypoint.
func (c *Client) CreatePromptFile(ctx context.Context, workspace, name, version, fileName, content string, isEntrypoint bool) (*PromptFile, error) {
	var resp struct {
		File *PromptFile `json:"createPromptFile"`
	}
	args := map[string]any{
		"name":         fileName,
		"content":      content,
		"isEntrypoint": isEntrypoint,
	}
	err := c.Do(ctx, Request{
		Query: "mutation CreatePromptFile($promptName: ID!, $version: ID!, $args: CreatePromptFileInput!) { createPromptFile(promptName: $promptName, version: $version, args: $args) { name content isEntrypoint } }",
		Variables: map[string]any{
			"promptName": name,
			"version":    version,
			"args":       args,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.File == nil {
		return nil, errMissingData("createPromptFile")
	}
	return resp.File, nil
}

// UpdatePromptFile updates a file's content and/or renames it. Pass an empty
// string for any field to leave it unchanged. Callers must set at least one.
// The backend returns the parent PromptVersion (so the caller sees the full
// file list); callers that need just the changed file should look it up by
// the post-rename name.
func (c *Client) UpdatePromptFile(ctx context.Context, workspace, name, version, fileName, newContent, newName string) (*PromptVersion, error) {
	args := map[string]any{}
	if newContent != "" {
		args["content"] = newContent
	}
	if newName != "" {
		args["name"] = newName
	}
	var resp struct {
		Version *PromptVersion `json:"updatePromptFile"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdatePromptFile($promptName: ID!, $version: ID!, $fileName: ID!, $args: UpdatePromptFileInput!) { updatePromptFile(promptName: $promptName, version: $version, fileName: $fileName, args: $args) { " + promptVersionFields + " } }",
		Variables: map[string]any{
			"promptName": name,
			"version":    version,
			"fileName":   fileName,
			"args":       args,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("updatePromptFile")
	}
	return resp.Version, nil
}

// DeletePromptFile removes a file from a draft version.
func (c *Client) DeletePromptFile(ctx context.Context, workspace, name, version, fileName string) (bool, error) {
	var resp struct {
		Deleted bool `json:"deletePromptFile"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation DeletePromptFile($promptName: ID!, $version: ID!, $fileName: ID!) { deletePromptFile(promptName: $promptName, version: $version, fileName: $fileName) }",
		Variables: map[string]any{
			"promptName": name,
			"version":    version,
			"fileName":   fileName,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return false, err
	}
	return resp.Deleted, nil
}

// SetPromptFileEntrypoint toggles a file's entrypoint flag. The backend
// returns the parent PromptVersion so callers can see the full file list
// after the mutation.
func (c *Client) SetPromptFileEntrypoint(ctx context.Context, workspace, name, version, fileName string, isEntrypoint bool) (*PromptVersion, error) {
	var resp struct {
		Version *PromptVersion `json:"setPromptFileEntrypoint"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation SetPromptFileEntrypoint($promptName: ID!, $version: ID!, $fileName: ID!, $isEntrypoint: Boolean!) { setPromptFileEntrypoint(promptName: $promptName, version: $version, fileName: $fileName, isEntrypoint: $isEntrypoint) { " + promptVersionFields + " } }",
		Variables: map[string]any{
			"promptName":   name,
			"version":      version,
			"fileName":     fileName,
			"isEntrypoint": isEntrypoint,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("setPromptFileEntrypoint")
	}
	return resp.Version, nil
}
