package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/WTomas/sufleur-cli/internal/generator"
)

// Client defines the interface for communicating with the Sufleur GraphQL API.
type Client interface {
	ValidatePrompts(ctx context.Context, promptNames []string) error
	FetchPromptVersion(ctx context.Context, promptName, constraint string, status *PromptVersionStatus) (*generator.PromptData, error)
}

// JSON represents a JSON scalar from the GraphQL schema.
type JSON map[string]interface{}

// UnmarshalJSON implements json.Unmarshaler.
func (j *JSON) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*j = m
	return nil
}

// GetGraphQLType returns the GraphQL type name for this scalar.
func (j JSON) GetGraphQLType() string {
	return "JSON"
}

// PromptVersionStatus represents the status of a prompt version.
type PromptVersionStatus string

const (
	StatusDraft     PromptVersionStatus = "DRAFT"
	StatusPublished PromptVersionStatus = "PUBLISHED"
)

// GetGraphQLType returns the GraphQL type name for this enum.
// Pointer receiver is required so nil *PromptVersionStatus values
// can safely satisfy the GraphQLType interface without panicking.
func (s *PromptVersionStatus) GetGraphQLType() string {
	return "PromptVersionStatus"
}

// GraphQLID is a string that maps to the GraphQL ID type.
type GraphQLID string

// GetGraphQLType returns the GraphQL type name.
func (id GraphQLID) GetGraphQLType() string {
	return "ID"
}

// ValidationError is returned when one or more prompt names are not found.
type ValidationError struct {
	MissingPrompts []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("prompts not found: %s", strings.Join(e.MissingPrompts, ", "))
}

// fetchPromptVersionQuery is the struct-based query for the hasura GraphQL client.
type fetchPromptVersionQuery struct {
	Prompt struct {
		Description string
		Version     struct {
			Version                 string
			Status                  string
			Metadata                JSON `scalar:"true"`
			UserPromptInputSchema   JSON `scalar:"true"`
			SystemPromptInputSchema JSON `scalar:"true"`
			OutputSchema            JSON `scalar:"true"`
			Files                   []promptFileResponse
		} `graphql:"version(constraint: $constraint, status: $status)"`
	} `graphql:"prompt(promptName: $promptName)"`
}

type promptFileResponse struct {
	Name    string
	Content string
}
