package userapi

import (
	"context"
	"strings"
	"time"
)

// KnownProviders is the GraphQL LlmProvider enum, in display order. The wire
// form (both input args and output values) is the uppercase enum name, even
// though the eval YAML uses the lowercase variant.
var KnownProviders = []string{"ANTHROPIC", "OPENAI", "GOOGLE", "MISTRAL", "DEEPSEEK", "XAI", "GROQ", "TOGETHER"}

// NormalizeProvider upper-cases a provider token (accepting the lowercase YAML
// form or the uppercase enum) and reports whether it is a known provider.
func NormalizeProvider(raw string) (string, bool) {
	up := strings.ToUpper(strings.TrimSpace(raw))
	for _, p := range KnownProviders {
		if p == up {
			return up, true
		}
	}
	return up, false
}

// ProviderCredential is a workspace's configured AI provider credential. The
// secret is never returned — only the last four characters.
type ProviderCredential struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Name      string    `json:"name"`
	LastFour  string    `json:"lastFour"`
	CreatedAt time.Time `json:"createdAt"`
}

// SupportedModel is one model available for a configured provider.
type SupportedModel struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	DisplayName     string `json:"displayName"`
	ContextWindow   int    `json:"contextWindow"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
	Source          string `json:"source"`
}

// ListProviderCredentials returns the AI provider credentials configured for a
// workspace. The workspace is resolved from the X-Workspace header, so the
// workspace argument is required. Non-members get an empty list.
func (c *Client) ListProviderCredentials(ctx context.Context, workspace string) ([]ProviderCredential, error) {
	var resp struct {
		Workspace *struct {
			ProviderCredentials []ProviderCredential `json:"providerCredentials"`
		} `json:"workspace"`
	}
	err := c.Do(ctx, Request{
		Query:     "query Providers { workspace { providerCredentials { id provider name lastFour createdAt } } }",
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Workspace == nil {
		return nil, errMissingData("workspace")
	}
	return resp.Workspace.ProviderCredentials, nil
}

// AvailableModels lists the models available for a configured provider in a
// workspace. The backend hits the live provider API (decrypting the stored
// key), so this errors when the provider has no credential or the key is
// rejected. provider must be the uppercase enum name.
func (c *Client) AvailableModels(ctx context.Context, workspace, provider string) ([]SupportedModel, error) {
	var resp struct {
		Models []SupportedModel `json:"availableModels"`
	}
	err := c.Do(ctx, Request{
		Query:     "query AvailableModels($provider: LlmProvider!) { availableModels(provider: $provider) { id provider displayName contextWindow maxOutputTokens source } }",
		Variables: map[string]any{"provider": provider},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Models, nil
}
