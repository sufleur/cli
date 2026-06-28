package userapi

import (
	"context"
	"time"
)

// Dataset mirrors the queryable subset of the GraphQL Dataset type that the
// CLI surfaces. Visibility decodes to the enum value name ("PUBLIC" /
// "PRIVATE"). Versions is only populated by GetDataset.
type Dataset struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Visibility  string               `json:"visibility"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
	Versions    *DatasetVersionsPage `json:"versions,omitempty"`
}

// DatasetsPage is the response shape of the `datasets` query.
type DatasetsPage struct {
	Data  []Dataset `json:"data"`
	Total int       `json:"total"`
}

const datasetFields = "id name description visibility createdAt updatedAt"

// GetDataset fetches a single dataset by its bare name, including a page of its
// versions (up to 100, newest-resolved order). The workspace is sent via the
// X-Workspace header.
func (c *Client) GetDataset(ctx context.Context, workspace, name string) (*Dataset, error) {
	var resp struct {
		Dataset *Dataset `json:"dataset"`
	}
	err := c.Do(ctx, Request{
		Query: "query GetDataset($datasetName: ID!, $pagination: PaginationArgs!) { dataset(datasetName: $datasetName) { " +
			datasetFields + " versions(pagination: $pagination) { data { " + datasetVersionListFields + " } total } } }",
		Variables: map[string]any{
			"datasetName": name,
			"pagination":  map[string]int{"take": 100, "skip": 0},
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset == nil {
		return nil, errMissingData("dataset")
	}
	return resp.Dataset, nil
}

// ListDatasets paginates through datasets in the workspace. take and skip map
// directly to PaginationArgs. search is optional.
func (c *Client) ListDatasets(ctx context.Context, workspace, search string, take, skip int) (*DatasetsPage, error) {
	vars := map[string]any{
		"pagination": map[string]int{"take": take, "skip": skip},
	}
	if search != "" {
		vars["search"] = search
	}
	var resp struct {
		Datasets *DatasetsPage `json:"datasets"`
	}
	err := c.Do(ctx, Request{
		Query:     "query ListDatasets($pagination: PaginationArgs!, $search: String) { datasets(pagination: $pagination, search: $search) { data { " + datasetFields + " } total } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Datasets == nil {
		return nil, errMissingData("datasets")
	}
	return resp.Datasets, nil
}

// CreateDataset creates a new dataset (and its initial draft version).
// description and visibility are optional; pass empty strings to omit them
// (the backend defaults visibility to PRIVATE). visibility is the enum value
// name, "PUBLIC" or "PRIVATE".
func (c *Client) CreateDataset(ctx context.Context, workspace, name, description, visibility string) (*Dataset, error) {
	vars := map[string]any{"name": name}
	if description != "" {
		vars["description"] = description
	}
	if visibility != "" {
		vars["visibility"] = visibility
	}
	var resp struct {
		Dataset *Dataset `json:"createDataset"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation CreateDataset($name: String!, $description: String, $visibility: DatasetVisibility) { createDataset(name: $name, description: $description, visibility: $visibility) { " + datasetFields + " } }",
		Variables: vars,
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset == nil {
		return nil, errMissingData("createDataset")
	}
	return resp.Dataset, nil
}

// UpdateDataset replaces the description on an existing dataset. Visibility
// changes use UpdateDatasetVisibility (a separate, distinctly-permissioned
// mutation).
func (c *Client) UpdateDataset(ctx context.Context, workspace, name, description string) (*Dataset, error) {
	var resp struct {
		Dataset *Dataset `json:"updateDataset"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdateDataset($datasetName: ID!, $args: UpdateDatasetArgs!) { updateDataset(datasetName: $datasetName, args: $args) { " + datasetFields + " } }",
		Variables: map[string]any{
			"datasetName": name,
			"args":        map[string]any{"description": description},
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset == nil {
		return nil, errMissingData("updateDataset")
	}
	return resp.Dataset, nil
}

// UpdateDatasetVisibility flips a dataset between PUBLIC and PRIVATE. visibility
// is the enum value name, "PUBLIC" or "PRIVATE".
func (c *Client) UpdateDatasetVisibility(ctx context.Context, workspace, name, visibility string) (*Dataset, error) {
	var resp struct {
		Dataset *Dataset `json:"updateDatasetVisibility"`
	}
	err := c.Do(ctx, Request{
		Query: "mutation UpdateDatasetVisibility($datasetName: ID!, $visibility: DatasetVisibility!) { updateDatasetVisibility(datasetName: $datasetName, visibility: $visibility) { " + datasetFields + " } }",
		Variables: map[string]any{
			"datasetName": name,
			"visibility":  visibility,
		},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Dataset == nil {
		return nil, errMissingData("updateDatasetVisibility")
	}
	return resp.Dataset, nil
}
