package userapi

import (
	"context"
	"time"
)

// DatasetVersion mirrors the queryable subset of the GraphQL DatasetVersion
// type that the CLI surfaces. Schema is a JSON scalar (always an object, "{}"
// when unset). ByteSize is a Float on the wire (gzipped blob size). Validation
// and CasesDownloadURL are only populated by the queries that request them.
type DatasetVersion struct {
	Version          string             `json:"version"`
	Status           string             `json:"status"`
	Schema           map[string]any     `json:"schema"`
	CaseCount        int                `json:"caseCount"`
	ByteSize         float64            `json:"byteSize"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
	Validation       *DatasetValidation `json:"validation,omitempty"`
	CasesDownloadURL string             `json:"casesDownloadUrl,omitempty"`
}

// DatasetValidation is the live validation report for a dataset version's cases
// against its current schema.
type DatasetValidation struct {
	Valid      bool                   `json:"valid"`
	CaseCount  int                    `json:"caseCount"`
	Violations []DatasetCaseViolation `json:"violations"`
}

// DatasetCaseViolation is one schema violation found in a specific case.
type DatasetCaseViolation struct {
	CaseIndex  int    `json:"caseIndex"`
	Constraint string `json:"constraint"`
	Message    string `json:"message"`
}

// DatasetVersionsPage is the response shape of Dataset.versions.
type DatasetVersionsPage struct {
	Data  []DatasetVersion `json:"data"`
	Total int              `json:"total"`
}

// datasetVersionListFields is the light projection used when listing versions
// (no schema body, no validation, no signed URL).
const datasetVersionListFields = "version status caseCount byteSize createdAt updatedAt"

const datasetValidationFields = "validation { valid caseCount violations { caseIndex constraint message } }"

// datasetVersionFields includes the schema body and the live validation report
// (used by mutations and `version get`).
const datasetVersionFields = "version status schema caseCount byteSize createdAt updatedAt " + datasetValidationFields

// CreateDatasetVersion creates a new draft version of an existing dataset,
// carrying forward the schema and cases of the latest published version. The
// backend rejects this if a draft already exists.
func (c *Client) CreateDatasetVersion(ctx context.Context, workspace, name string) (*DatasetVersion, error) {
	var resp struct {
		Version *DatasetVersion `json:"createDatasetVersion"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreateDatasetVersion($datasetName: ID!) { createDatasetVersion(datasetName: $datasetName) { " + datasetVersionFields + " } }",
		Variables: map[string]any{"datasetName": name},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("createDatasetVersion")
	}
	return resp.Version, nil
}

// GetDatasetVersion resolves a single version by constraint ("1.2.3", "^1.0",
// "draft", etc.) and returns its schema and live validation report.
func (c *Client) GetDatasetVersion(ctx context.Context, workspace, name, constraint string) (*DatasetVersion, error) {
	var resp struct {
		Dataset struct {
			Version *DatasetVersion `json:"version"`
		} `json:"dataset"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetDatasetVersion($datasetName: ID!, $constraint: String!) { dataset(datasetName: $datasetName) { version(constraint: $constraint) { " + datasetVersionFields + " } } }",
		Variables: map[string]any{"datasetName": name, "constraint": constraint},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset.Version == nil {
		return nil, errMissingData("version")
	}
	return resp.Dataset.Version, nil
}

// GetDatasetVersionForDownload resolves a version and returns its schema plus a
// signed URL to download its cases blob (empty when the version has no cases).
// It skips the live validation report to avoid re-validating the whole blob.
func (c *Client) GetDatasetVersionForDownload(ctx context.Context, workspace, name, constraint string) (*DatasetVersion, error) {
	var resp struct {
		Dataset struct {
			Version *DatasetVersion `json:"version"`
		} `json:"dataset"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetDatasetVersionDownload($datasetName: ID!, $constraint: String!) { dataset(datasetName: $datasetName) { version(constraint: $constraint) { version status schema caseCount byteSize createdAt updatedAt casesDownloadUrl } } }",
		Variables: map[string]any{"datasetName": name, "constraint": constraint},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset.Version == nil {
		return nil, errMissingData("version")
	}
	return resp.Dataset.Version, nil
}

// ListDatasetVersions paginates the versions of a dataset. status filters to
// DRAFT or PUBLISHED (enum value name) when non-empty.
func (c *Client) ListDatasetVersions(ctx context.Context, workspace, name, status string, take, skip int) (*DatasetVersionsPage, error) {
	vars := map[string]any{
		"datasetName": name,
		"pagination":  map[string]int{"take": take, "skip": skip},
	}
	if status != "" {
		vars["status"] = status
	}
	var resp struct {
		Dataset struct {
			Versions *DatasetVersionsPage `json:"versions"`
		} `json:"dataset"`
	}
	err := c.Do(ctx, Request{
		Query:     "query ListDatasetVersions($datasetName: ID!, $pagination: PaginationArgs!, $status: DatasetVersionStatus) { dataset(datasetName: $datasetName) { versions(pagination: $pagination, status: $status) { data { " + datasetVersionListFields + " } total } } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset.Versions == nil {
		return nil, errMissingData("versions")
	}
	return resp.Dataset.Versions, nil
}

// SetDatasetSchema replaces the JSON Schema on a draft version and returns the
// updated version including its refreshed validation report.
func (c *Client) SetDatasetSchema(ctx context.Context, workspace, name, version string, schema map[string]any) (*DatasetVersion, error) {
	var resp struct {
		Version *DatasetVersion `json:"setDatasetSchema"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation SetDatasetSchema($datasetName: ID!, $version: ID!, $schema: JSON!) { setDatasetSchema(datasetName: $datasetName, version: $version, schema: $schema) { " + datasetVersionFields + " } }",
		Variables: map[string]any{
			"datasetName": name,
			"version":     version,
			"schema":      schema,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Version == nil {
		return nil, errMissingData("setDatasetSchema")
	}
	return resp.Version, nil
}

// DeleteDatasetVersion deletes a draft version. The backend rejects deleting
// published versions or any but the latest version, surfacing a GraphQL error.
func (c *Client) DeleteDatasetVersion(ctx context.Context, workspace, name, version string) (bool, error) {
	var resp struct {
		Deleted bool `json:"deleteDatasetVersion"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation DeleteDatasetVersion($datasetName: ID!, $version: ID!) { deleteDatasetVersion(datasetName: $datasetName, version: $version) }",
		Variables: map[string]any{"datasetName": name, "version": version},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return false, err
	}
	return resp.Deleted, nil
}
