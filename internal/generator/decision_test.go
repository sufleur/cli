package generator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
)

const specJSON = `{"questions":{"isUrgent":{"type":"noul","criteria":{"true":"Time-sensitive"}},"department":{"type":"choice","criteria":{"technical":null,"billing":"Payments","sales":{"scope":["pricing"]}}},"frustration":{"type":"score","criteria":["Calm","Frustrated","Very angry"]},"about":{"type":"choice","criteria":{"none":null},"optionCriteria":{"what":"about {{{name}}}"}},"bare":{"type":"choice","criteria":{},"optionCriteria":null}}}`

func TestDecisionSpecPreservesAuthoredOrder(t *testing.T) {
	var spec DecisionSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, q := range spec.Questions {
		ids = append(ids, q.ID)
	}
	if want := []string{"isUrgent", "department", "frustration", "about", "bare"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("question order = %v, want %v", ids, want)
	}
	if got := spec.Questions[1].ChoiceOptions(); !reflect.DeepEqual(got, []string{"technical", "billing", "sales"}) {
		t.Fatalf("choice options = %v", got)
	}
	if got := spec.Questions[2].ScoreLevels(); got != 3 {
		t.Fatalf("score levels = %d", got)
	}
	if got := spec.EntrypointNames(); !reflect.DeepEqual(got, []string{"isUrgent", "department", "frustration", "about", "bare"}) {
		t.Fatalf("entrypoints = %v", got)
	}
	if spec.Questions[1].IsOpenChoice() || !spec.Questions[3].IsOpenChoice() || !spec.Questions[4].IsOpenChoice() {
		t.Fatalf("open choices misdetected: %+v", spec.Questions)
	}
}

func TestDecisionSpecRoundTripsDeterministically(t *testing.T) {
	var spec DecisionSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != specJSON {
		t.Fatalf("round trip changed the spec:\n got %s\nwant %s", out, specJSON)
	}
}

func TestDecisionSpecIgnoresLegacyStateFile(t *testing.T) {
	var spec DecisionSpec
	if err := json.Unmarshal([]byte(`{"stateFile":"state","questions":{"q":{"type":"noul"}}}`), &spec); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(spec)
	if string(out) != `{"questions":{"q":{"type":"noul"}}}` {
		t.Fatalf("got %s", out)
	}
}

func TestWholeValueVariable(t *testing.T) {
	cases := map[string]string{
		"{{ticket}}":            "ticket",
		" {{{ ticket.body }}} ": "ticket.body",
		"{{& ticket}}":          "ticket",
		"{{a}} {{b}}":           "",
		"Hi {{name}}":           "",
		"{{#items}}x{{/items}}": "",
		"{{> policy}}":          "",
		"{{@doc ticket.body}}":  "",
	}
	for in, want := range cases {
		if got := WholeValueVariable(in); got != want {
			t.Errorf("WholeValueVariable(%q) = %q, want %q", in, got, want)
		}
	}
}

// Mirrors the backend structured-template.spec.ts fixture.
func TestParseAndRenderStructured(t *testing.T) {
	tree, err := ParseStructured(`
ticket: "{{ticket}}"
customer:
  tier: "{{tier}}"
  note: "VIP since {{since}}"
  vip: true
limit: 3
refund_policy: "{{> refund_policy}}"
tags: ["{{first}}", fixed]
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tree.Templates(), "\n"); got != "{{ticket}}\n{{tier}}\nVIP since {{since}}\n{{> refund_policy}}\n{{first}}\nfixed" {
		t.Fatalf("templates in document order = %q", got)
	}

	view := map[string]any{
		"ticket": map[string]any{"subject": "Duplicate charge", "messages": []any{map[string]any{"text": "He said \"refund\", then\nleft"}}},
		"tier":   "gold",
		"since":  2019,
		"first":  "a: b",
	}
	partials := &mustache.StaticProvider{Partials: map[string]string{"refund_policy": "Refunds within 30 days."}}
	rendered, err := RenderStructured(tree, view, func(tmpl string) (string, error) {
		return mustache.RenderPartials(tmpl, partials, view)
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(rendered)
	want := `{"ticket":{"messages":[{"text":"He said \"refund\", then\nleft"}],"subject":"Duplicate charge"},"customer":{"tier":"gold","note":"VIP since 2019","vip":true},"limit":3,"refund_policy":"Refunds within 30 days.","tags":["a: b","fixed"]}`
	if string(out) != want {
		t.Fatalf("rendered:\n got %s\nwant %s", out, want)
	}
}

func TestRenderStructuredMissingWholeValueIsNull(t *testing.T) {
	tree, err := ParseStructured(`count: "{{n}}"` + "\nmissing: \"{{nope}}\"")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderStructured(tree, map[string]any{"n": 4}, func(s string) (string, error) { return s, nil })
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(rendered)
	if string(out) != `{"count":4,"missing":null}` {
		t.Fatalf("got %s", out)
	}
}

func TestParseStructuredRejectsUnquotedTagsAndDuplicates(t *testing.T) {
	if _, err := ParseStructured("ticket: {{ticket}}"); err == nil || !strings.Contains(err.Error(), "must be quoted") {
		t.Fatalf("unquoted tag: err = %v", err)
	}
	if _, err := ParseStructured("a: 1\na: 2"); err == nil {
		t.Fatal("duplicate keys must be rejected")
	}
}
