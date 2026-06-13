package promptref

import (
	"fmt"
	"strings"
)

// PromptRef represents a parsed workspace-scoped prompt reference. If the
// input string includes an "@version" suffix, Version holds it; otherwise it
// is empty.
//
// A "+" prefix on the name segment (e.g. "@ws/+onboarding") marks the
// reference as a collection rather than a prompt. The "+" is a CLI-only type
// marker: IsCollection is set and Name holds the bare backend name with the
// "+" stripped, while Raw preserves the original input. The marker can never
// collide with a real name because prompt and collection names disallow "+".
type PromptRef struct {
	Workspace    string // "wtomas" (no @ prefix)
	Name         string // "my-prompt" (collection marker "+" stripped)
	Version      string // "1.2.3" or "draft", "" if not specified
	IsCollection bool   // true when the name segment was "+"-prefixed
	Raw          string // the original string passed in
}

// Parse splits a prompt key like "@workspace/prompt-name" into a PromptRef.
// Returns an error if the input contains an "@version" suffix; use ParseRef
// for inputs that may include one.
func Parse(key string) (PromptRef, error) {
	ref, err := parse(key)
	if err != nil {
		return PromptRef{}, err
	}
	if ref.Version != "" {
		return PromptRef{}, fmt.Errorf("prompt key %q has unexpected version suffix", key)
	}
	return ref, nil
}

// ParseRef parses "@workspace/name" or "@workspace/name@version". Version is
// returned in PromptRef.Version when present, empty otherwise. The parser
// accepts any non-empty version string and does not validate semver — let
// the server reject unsupported values.
func ParseRef(key string) (PromptRef, error) {
	return parse(key)
}

func parse(key string) (PromptRef, error) {
	if !strings.HasPrefix(key, "@") {
		return PromptRef{}, fmt.Errorf("prompt key %q must start with @", key)
	}

	// Strip the optional "@version" suffix first so a version that contains
	// a "/" (unlikely but harmless to allow) doesn't confuse the slash search.
	body := key
	version := ""
	if at := strings.LastIndex(key, "@"); at > 0 {
		body = key[:at]
		version = key[at+1:]
		if version == "" {
			return PromptRef{}, fmt.Errorf("prompt key %q has empty version after @", key)
		}
	}

	slashIdx := strings.Index(body, "/")
	if slashIdx == -1 {
		return PromptRef{}, fmt.Errorf("prompt key %q must contain /", key)
	}

	workspace := body[1:slashIdx]
	name := body[slashIdx+1:]

	if workspace == "" {
		return PromptRef{}, fmt.Errorf("prompt key %q has empty workspace", key)
	}
	if name == "" {
		return PromptRef{}, fmt.Errorf("prompt key %q has empty prompt name", key)
	}

	// A leading "+" on the name segment marks a collection. Strip it so Name
	// is the bare backend identifier; Raw keeps the original form.
	isCollection := false
	if strings.HasPrefix(name, "+") {
		isCollection = true
		name = name[1:]
		if name == "" {
			return PromptRef{}, fmt.Errorf("collection key %q has empty name after %q", key, "+")
		}
	}

	return PromptRef{
		Workspace:    workspace,
		Name:         name,
		Version:      version,
		IsCollection: isCollection,
		Raw:          key,
	}, nil
}
