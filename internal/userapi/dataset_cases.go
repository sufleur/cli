package userapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// IngestResponse is the JSON body returned by the dataset cases ingest REST
// endpoint. schemaInferred is true when the upload established the version's
// schema (the first upload to a fresh draft). enumSuggestions are advisory
// only — the backend never auto-applies them.
type IngestResponse struct {
	VersionID       string             `json:"versionId"`
	CaseCount       int                `json:"caseCount"`
	ByteSize        int64              `json:"byteSize"`
	Schema          map[string]any     `json:"schema"`
	SchemaInferred  bool               `json:"schemaInferred"`
	EnumSuggestions []EnumSuggestion   `json:"enumSuggestions"`
	Validation      *DatasetValidation `json:"validation"`
}

// EnumSuggestion proposes that a cardinality-bounded field be constrained to an
// enum. Values holds the distinct observed values.
type EnumSuggestion struct {
	Field  string `json:"field"`
	Values []any  `json:"values"`
}

// IngestCases uploads a cases file (JSONL/JSON/CSV) to a draft version via the
// REST ingest endpoint. The filename is preserved so the backend can detect the
// format from its extension. It mirrors the GraphQL client's auth: bearer token,
// X-Client, and the X-Workspace header.
func (c *Client) IngestCases(ctx context.Context, workspace, name, version, filename string, r io.Reader) (*IngestResponse, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("building upload: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, fmt.Errorf("reading cases: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("finalising upload: %w", err)
	}

	endpoint := c.base + "/dataset/" + url.PathEscape(name) + "/versions/" + url.PathEscape(version) + "/cases"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("X-Client", "cli")
	if workspace != "" {
		req.Header.Set("X-Workspace", workspace)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cases upload: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrBearerRejected
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("cases upload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out IngestResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parsing ingest response: %w", err)
	}
	return &out, nil
}

// DownloadCases fetches a version's gzipped JSONL cases blob from a pre-signed
// URL and returns the decompressed JSONL bytes. The URL already carries its own
// authorization, so no headers are added.
func (c *Client) DownloadCases(ctx context.Context, signedURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading cases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cases download returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decompressing cases: %w", err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("reading cases: %w", err)
	}
	return data, nil
}
