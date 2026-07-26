package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/config"
)

// fakeRegistry is a minimal stand-in for the Sufleur GraphQL API, used to
// integration-test the add/update command flows end-to-end (through the real
// fetcher.Client and resolver.Resolver) without a live backend. It switches
// on distinctive substrings of the outgoing query text to answer the three
// shapes the fetcher client sends: ValidatePrompts, FetchPromptVersion, and
// ListCollectionPrompts.
type fakeRegistry struct {
	t *testing.T

	// versions maps a bare prompt name to the version FetchPromptVersion
	// should return. A name absent from this map causes the fake server to
	// return a null version, which the fetcher surfaces as
	// *fetcher.NoPublishedVersionError.
	versions map[string]string

	// collections maps a collection name to its member prompt names.
	collections map[string][]string
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	return &fakeRegistry{
		t:           t,
		versions:    map[string]string{},
		collections: map[string][]string{},
	}
}

// start launches the fake server and returns its base URL. It is closed
// automatically via t.Cleanup.
func (f *fakeRegistry) start() string {
	f.t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(f.handle))
	f.t.Cleanup(ts.Close)
	return ts.URL
}

func (f *fakeRegistry) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Fatalf("fakeRegistry: reading request body: %v", err)
	}

	var req struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		f.t.Fatalf("fakeRegistry: unmarshaling request: %v (body: %s)", err, body)
	}

	w.Header().Set("Content-Type", "application/json")

	switch {
	case strings.Contains(req.Query, "ValidatePrompts"):
		// Always succeeds: every name this suite cares about is treated as
		// existing in the registry. Nothing in these tests exercises the
		// "prompt does not exist at all" path.
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})

	case strings.Contains(req.Query, "promptCollection(name:"):
		name, _ := req.Variables["name"].(string)
		members := f.collections[name]
		prompts := make([]map[string]any, len(members))
		for i, m := range members {
			prompts[i] = map[string]any{"name": m}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"promptCollection": map[string]any{"prompts": prompts},
			},
		})

	case strings.Contains(req.Query, "prompt(promptName:"):
		name, _ := req.Variables["promptName"].(string)
		var version any
		if v, ok := f.versions[name]; ok {
			version = map[string]any{
				"version":      v,
				"status":       "PUBLISHED",
				"metadata":     map[string]any{},
				"outputSchema": nil,
				"modelConfig":  nil,
				"files": []map[string]any{
					{
						"name":           "main.txt",
						"content":        "Hello from " + name,
						"isEntrypoint":   true,
						"inputSchema":    nil,
						"schemaWarnings": []any{},
					},
				},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"prompt": map[string]any{
					"description": "Test " + name,
					"version":     version,
				},
			},
		})

	default:
		f.t.Fatalf("fakeRegistry: unexpected query: %s", req.Query)
	}
}

// writeSufleurYAML writes a minimal, anonymous-workspace sufleur.yaml (no
// api_keys — public prompts only) into dir/sufleur.yaml with the given
// prompts map, and returns to the caller's original working directory via
// t.Cleanup.
func writeSufleurYAML(t *testing.T, prompts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if prompts == nil {
		prompts = map[string]string{}
	}
	cfg := config.SufleurConfig{
		Prompts: prompts,
		Output:  config.OutputConfig{Language: "typescript", File: "./out/prompts.ts"},
	}
	if err := config.Save("sufleur.yaml", cfg); err != nil {
		t.Fatalf("writing sufleur.yaml: %v", err)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}
