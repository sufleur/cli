package userapi

import "context"

// PromptToolPin is one tool contract pinned by a prompt version.
//
// Alias is the wire name the model sees. It belongs to the pin, not the tool:
// the same contract can be pinned under different names by different prompts,
// which is how two tools that share a bare name are told apart.
type PromptToolPin struct {
	Alias       string            `json:"alias"`
	ToolVersion PinnedToolVersion `json:"toolVersion"`
}

// PinnedToolVersion is the contract a pin resolves to, as returned inside a
// prompt version.
type PinnedToolVersion struct {
	Version          string     `json:"version"`
	Status           string     `json:"status"`
	ModelDescription string     `json:"modelDescription"`
	Tool             PinnedTool `json:"tool"`
}

// PinnedTool identifies the pinned tool. Workspace is selected because a prompt
// may pin a tool owned by another workspace.
type PinnedTool struct {
	Name      string `json:"name"`
	Workspace struct {
		Name string `json:"name"`
	} `json:"workspace"`
}

// Ref renders the pin's registry reference, e.g. "@vendor/web-search".
func (p PinnedTool) Ref() string { return "@" + p.Workspace.Name + "/" + p.Name }

const promptToolFields = "tools { alias toolVersion { version status modelDescription tool { name workspace { name } } } }"

// toolIdentifier is the {workspace, name} shape the link mutations take.
func toolIdentifier(workspace, name string) map[string]any {
	return map[string]any{"workspace": workspace, "name": name}
}

// GetPromptVersionTools returns the tool contracts a prompt version pins.
//
// Pins the caller cannot read are filtered out by the server, so this is what
// the caller's credentials can see rather than necessarily the whole set.
func (c *Client) GetPromptVersionTools(ctx context.Context, workspace, name, constraint string) ([]PromptToolPin, error) {
	var resp struct {
		Prompt *struct {
			Version *struct {
				Tools []PromptToolPin `json:"tools"`
			} `json:"version"`
		} `json:"prompt"`
	}
	err := c.Do(ctx, Request{
		Query: "query GetPromptVersionTools($promptName: ID!, $constraint: String!) { prompt(promptName: $promptName) { version(constraint: $constraint) { " +
			promptToolFields + " } } }",
		Variables: map[string]any{"promptName": name, "constraint": constraint},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Prompt == nil || resp.Prompt.Version == nil {
		return nil, errMissingData("prompt.version")
	}
	return resp.Prompt.Version.Tools, nil
}

// LinkTool pins a tool version to a draft prompt version.
//
// toolVersion may be a semver constraint; the server resolves it to a concrete
// version at link time, so the pin never moves afterwards. An empty alias lets
// the server default it to the tool's own name.
func (c *Client) LinkTool(ctx context.Context, workspace, promptName, promptVersion, toolWorkspace, toolName, toolVersion, alias string) error {
	tool := toolIdentifier(toolWorkspace, toolName)
	tool["version"] = toolVersion

	args := map[string]any{"tool": tool}
	if alias != "" {
		args["alias"] = alias
	}

	return c.Do(ctx, Request{
		Query: "mutation LinkTool($promptName: ID!, $version: ID!, $args: LinkToolInput!) { linkTool(promptName: $promptName, version: $version, args: $args) { version } }",
		Variables: map[string]any{
			"promptName": promptName,
			"version":    promptVersion,
			"args":       args,
		},
		Workspace: workspace,
	}, nil)
}

// UpdateToolLink renames the wire name an existing pin is exposed under.
func (c *Client) UpdateToolLink(ctx context.Context, workspace, promptName, promptVersion, toolWorkspace, toolName, alias string) error {
	return c.Do(ctx, Request{
		Query: "mutation UpdateToolLink($promptName: ID!, $version: ID!, $tool: ToolIdentifierInput!, $alias: String!) { updateToolLink(promptName: $promptName, version: $version, tool: $tool, alias: $alias) { version } }",
		Variables: map[string]any{
			"promptName": promptName,
			"version":    promptVersion,
			"tool":       toolIdentifier(toolWorkspace, toolName),
			"alias":      alias,
		},
		Workspace: workspace,
	}, nil)
}

// UnlinkTool removes a pin from a draft prompt version.
func (c *Client) UnlinkTool(ctx context.Context, workspace, promptName, promptVersion, toolWorkspace, toolName string) error {
	return c.Do(ctx, Request{
		Query: "mutation UnlinkTool($promptName: ID!, $version: ID!, $tool: ToolIdentifierInput!) { unlinkTool(promptName: $promptName, version: $version, tool: $tool) { version } }",
		Variables: map[string]any{
			"promptName": promptName,
			"version":    promptVersion,
			"tool":       toolIdentifier(toolWorkspace, toolName),
		},
		Workspace: workspace,
	}, nil)
}
