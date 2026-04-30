package promptref

import (
	"fmt"
	"strings"
)

// PromptRef represents a parsed workspace-scoped prompt reference.
type PromptRef struct {
	Workspace string // "wtomas" (no @ prefix)
	Name      string // "my-prompt"
	Raw       string // "@wtomas/my-prompt" (original key)
}

// Parse splits a prompt key like "@workspace/prompt-name" into a PromptRef.
func Parse(key string) (PromptRef, error) {
	if !strings.HasPrefix(key, "@") {
		return PromptRef{}, fmt.Errorf("prompt key %q must start with @", key)
	}

	slashIdx := strings.Index(key, "/")
	if slashIdx == -1 {
		return PromptRef{}, fmt.Errorf("prompt key %q must contain /", key)
	}

	workspace := key[1:slashIdx]
	name := key[slashIdx+1:]

	if workspace == "" {
		return PromptRef{}, fmt.Errorf("prompt key %q has empty workspace", key)
	}
	if name == "" {
		return PromptRef{}, fmt.Errorf("prompt key %q has empty prompt name", key)
	}

	return PromptRef{
		Workspace: workspace,
		Name:      name,
		Raw:       key,
	}, nil
}

// CacheKey returns a filesystem-safe key for caching (replaces / with __).
func CacheKey(ref PromptRef) string {
	return strings.ReplaceAll(ref.Raw, "/", "__")
}
