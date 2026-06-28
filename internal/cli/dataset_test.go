package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/userapi"
)

func TestParseDatasetRef(t *testing.T) {
	t.Run("rejects collection marker", func(t *testing.T) {
		if _, err := parseDatasetRef("@acme/+orders", false); err == nil {
			t.Fatalf("expected error for collection ref")
		}
	})
	t.Run("requires version when asked", func(t *testing.T) {
		if _, err := parseDatasetRef("@acme/orders", true); err == nil {
			t.Fatalf("expected error when version missing")
		}
	})
	t.Run("parses workspace, name, version", func(t *testing.T) {
		ref, err := parseDatasetRef("@acme/orders@1.2.3", true)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if ref.Workspace != "acme" || ref.Name != "orders" || ref.Version != "1.2.3" {
			t.Errorf("got %+v", ref)
		}
	})
}

func TestCasesUploadFilename(t *testing.T) {
	tests := []struct {
		path, format, want string
		wantErr            bool
	}{
		{"data/cases.csv", "", "cases.csv", false},
		{"-", "", "cases.jsonl", false},
		{"-", "csv", "cases.csv", false},
		{"whatever.txt", "json", "cases.json", false},
		{"-", "yaml", "", true},
	}
	for _, tc := range tests {
		got, err := casesUploadFilename(tc.path, tc.format)
		if tc.wantErr {
			if err == nil {
				t.Errorf("casesUploadFilename(%q,%q): expected error", tc.path, tc.format)
			}
			continue
		}
		if err != nil {
			t.Errorf("casesUploadFilename(%q,%q): %v", tc.path, tc.format, err)
		}
		if got != tc.want {
			t.Errorf("casesUploadFilename(%q,%q) = %q, want %q", tc.path, tc.format, got, tc.want)
		}
	}
}

func TestWriteDatasetDump(t *testing.T) {
	dir := t.TempDir()
	d := &userapi.Dataset{Name: "orders", Description: "the orders", Visibility: "PUBLIC"}
	v := &userapi.DatasetVersion{Version: "1.0.0", Status: "PUBLISHED", CaseCount: 2, Schema: map[string]any{"type": "object"}}
	cases := []byte("{\"id\":1}\n{\"id\":2}\n")

	if err := writeDatasetDump(dir, d, v, cases); err != nil {
		t.Fatalf("writeDatasetDump: %v", err)
	}

	schema, err := os.ReadFile(filepath.Join(dir, "schema.json"))
	if err != nil || !strings.Contains(string(schema), `"type": "object"`) {
		t.Errorf("schema.json = %q (err %v)", schema, err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "cases.jsonl"))
	if err != nil || string(got) != string(cases) {
		t.Errorf("cases.jsonl = %q (err %v)", got, err)
	}
	meta, err := os.ReadFile(filepath.Join(dir, "dataset.yaml"))
	if err != nil {
		t.Fatalf("dataset.yaml: %v", err)
	}
	for _, want := range []string{"name: orders", "visibility: PUBLIC", "version: 1.0.0", "caseCount: 2"} {
		if !strings.Contains(string(meta), want) {
			t.Errorf("dataset.yaml missing %q:\n%s", want, meta)
		}
	}
}

func TestSchemaSummary(t *testing.T) {
	if got := schemaSummary(nil); got != "none" {
		t.Errorf("nil schema = %q", got)
	}
	if got := schemaSummary(map[string]any{"properties": map[string]any{"a": 1, "b": 2}}); got != "2 properties" {
		t.Errorf("got %q", got)
	}
	if got := schemaSummary(map[string]any{"properties": map[string]any{"a": 1}}); got != "1 property" {
		t.Errorf("got %q", got)
	}
}
