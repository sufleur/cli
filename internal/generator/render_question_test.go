package generator

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
)

type renderCase struct {
	Name         string          `json:"name"`
	Question     json.RawMessage `json:"question"`
	Instructions struct {
		Format  string `json:"format"`
		Content string `json:"content"`
	} `json:"instructions"`
	Partials      map[string]string `json:"partials"`
	Inputs        map[string]any    `json:"inputs"`
	Options       json.RawMessage   `json:"options"`
	Expected      json.RawMessage   `json:"expected"`
	ExpectedError string            `json:"expectedError"`
	Escaped       map[string]string `json:"escaped"`
}

// loadRenderCases reads the rendering cases shared with the backend and the
// generated runtimes (a byte-identical copy lives in the monorepo).
func loadRenderCases(t *testing.T) []renderCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/decision-render-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []renderCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	return file.Cases
}

func renderCaseWithGo(t *testing.T, c renderCase) (json.RawMessage, error) {
	t.Helper()
	var spec DecisionSpec
	wrapped := append(append([]byte(`{"questions":{"q":`), c.Question...), '}', '}')
	if err := json.Unmarshal(wrapped, &spec); err != nil {
		t.Fatal(err)
	}
	instructions := QuestionInstructions{Template: c.Instructions.Content}
	if c.Instructions.Format == "yaml" {
		tree, err := ParseStructured(c.Instructions.Content)
		if err != nil {
			t.Fatal(err)
		}
		instructions = QuestionInstructions{YAML: true, Tree: tree}
	}
	var options []DecisionOption
	if len(c.Options) > 0 {
		var err error
		if options, err = OrderedOptions(c.Options); err != nil {
			t.Fatal(err)
		}
	}
	provider := &mustache.StaticProvider{Partials: c.Partials}
	return RenderDecisionQuestion(spec.Questions[0], instructions, c.Inputs, options,
		func(template string, view map[string]any) (string, error) {
			return mustache.RenderPartials(template, provider, view)
		})
}

func TestRenderDecisionQuestionSharedCases(t *testing.T) {
	for _, c := range loadRenderCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			got, err := renderCaseWithGo(t, c)
			switch {
			case c.ExpectedError != "":
				if err == nil || !strings.Contains(err.Error(), c.ExpectedError) {
					t.Fatalf("error = %v, want one containing %q", err, c.ExpectedError)
				}
			case c.Escaped != nil:
				if err != nil {
					t.Fatal(err)
				}
				var out struct{ Instructions string }
				_ = json.Unmarshal(got, &out)
				if out.Instructions != c.Escaped["cbroglie"] {
					t.Fatalf("got %q, want %q", out.Instructions, c.Escaped["cbroglie"])
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				var want bytes.Buffer
				if err := json.Compact(&want, c.Expected); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want.Bytes()) {
					t.Fatalf("\n got %s\nwant %s", got, want.Bytes())
				}
			}
		})
	}
}
