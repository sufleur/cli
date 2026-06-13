package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sufleur/cli/internal/generator"
)

// Client defines the interface for communicating with the Sufleur GraphQL API.
type Client interface {
	ValidatePrompts(ctx context.Context, promptNames []string) error
	FetchPromptVersion(ctx context.Context, promptName, constraint string, status *PromptVersionStatus) (*generator.PromptData, error)
	ListCollectionPrompts(ctx context.Context, collectionName string) ([]string, error)
}

// JSON represents a JSON object scalar from the GraphQL schema.
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

// schemaWarning is a single warning entry returned by the backend per entrypoint.
type schemaWarning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// SchemaWarningList is a JSON-array scalar returned for PromptFile.schemaWarnings.
type SchemaWarningList []schemaWarning

// GetGraphQLType returns the GraphQL type name for this scalar.
func (s SchemaWarningList) GetGraphQLType() string {
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

// fetchPromptVersionResult is the shape of the resolved version sub-selection.
// It is referenced via pointer in fetchPromptVersionQuery so a null version
// (no match for the constraint) decodes to nil rather than zero-value confusion.
type fetchPromptVersionResult struct {
	Version      string
	Status       string
	Metadata     JSON `scalar:"true"`
	OutputSchema JSON `scalar:"true"`
	Files        []promptFileResponse
}

// fetchPromptVersionQuery is the struct-based query for the hasura GraphQL client.
type fetchPromptVersionQuery struct {
	Prompt struct {
		Description string
		Version     *fetchPromptVersionResult `graphql:"version(constraint: $constraint, status: $status)"`
	} `graphql:"prompt(promptName: $promptName)"`
}

// listCollectionPromptsQuery resolves the member prompts of a collection. The
// collection name carries no "+" marker here — that is a CLI-only concern.
type listCollectionPromptsQuery struct {
	PromptCollection struct {
		Prompts []struct {
			Name string
		}
	} `graphql:"promptCollection(name: $name)"`
}

type promptFileResponse struct {
	Name           string
	Content        string
	IsEntrypoint   bool
	InputSchema    JSON              `scalar:"true"`
	SchemaWarnings SchemaWarningList `scalar:"true"`
}
