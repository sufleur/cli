package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// KindSystemOne is the Prompt.kind value of decision prompts: typed
// noul / choice / score questions evaluated by a System-One model (TypeSafe
// Jev) against one state, instead of chat templates for a text-generating LLM.
const KindSystemOne = "SYSTEM_ONE"

// IsDecision reports whether the prompt is a SYSTEM_ONE decision prompt.
func (p PromptData) IsDecision() bool {
	return p.Kind == KindSystemOne && p.DecisionSpec != nil
}

// DecisionSpec is a SYSTEM_ONE version's output descriptor. Each question id
// is also the name of the entrypoint file holding its instructions. Questions
// keep the order they were authored in — the backend stores the spec as
// order-preserving JSON, and this type decodes it without going through a Go
// map, so generated code and dumps list questions (and choice options) in
// author order.
type DecisionSpec struct {
	StateFile string
	Questions []DecisionQuestion
}

// DecisionQuestion is one question. Criteria is kept as raw JSON so option
// order and structured (object/array) descriptions survive untouched.
type DecisionQuestion struct {
	ID       string
	Type     string // "noul" | "choice" | "score"
	Criteria json.RawMessage
}

// UnmarshalJSON decodes {stateFile?, questions: {id: {type, criteria?}}}
// preserving the order of the question keys.
func (s *DecisionSpec) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("decision spec: %w", err)
	}
	*s = DecisionSpec{}
	if raw, ok := top["stateFile"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &s.StateFile); err != nil {
			return fmt.Errorf("decision spec stateFile: %w", err)
		}
	}
	raw, ok := top["questions"]
	if !ok {
		return nil
	}
	return orderedObject(raw, func(id string, value json.RawMessage) error {
		var q struct {
			Type     string          `json:"type"`
			Criteria json.RawMessage `json:"criteria"`
		}
		if err := json.Unmarshal(value, &q); err != nil {
			return fmt.Errorf("decision spec question %q: %w", id, err)
		}
		criteria := q.Criteria
		if bytes.Equal(bytes.TrimSpace(criteria), []byte("null")) {
			criteria = nil
		}
		s.Questions = append(s.Questions, DecisionQuestion{ID: id, Type: q.Type, Criteria: criteria})
		return nil
	})
}

// MarshalJSON writes the spec back in the wire shape, in question order.
// Deterministic output keeps cache files and lockfile integrity hashes stable.
func (s DecisionSpec) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	if s.StateFile != "" {
		stateFile, _ := json.Marshal(s.StateFile)
		b.WriteString(`"stateFile":`)
		b.Write(stateFile)
		b.WriteByte(',')
	}
	b.WriteString(`"questions":{`)
	for i, q := range s.Questions {
		if i > 0 {
			b.WriteByte(',')
		}
		id, _ := json.Marshal(q.ID)
		typ, _ := json.Marshal(q.Type)
		b.Write(id)
		b.WriteString(`:{"type":`)
		b.Write(typ)
		if len(q.Criteria) > 0 {
			var compact bytes.Buffer
			if err := json.Compact(&compact, q.Criteria); err != nil {
				return nil, err
			}
			b.WriteString(`,"criteria":`)
			b.Write(compact.Bytes())
		}
		b.WriteByte('}')
	}
	b.WriteString("}}")
	return b.Bytes(), nil
}

// ChoiceOptions returns a choice question's options in authored order.
func (q DecisionQuestion) ChoiceOptions() []string {
	if q.Type != "choice" || len(q.Criteria) == 0 {
		return nil
	}
	var options []string
	_ = orderedObject(q.Criteria, func(key string, _ json.RawMessage) error {
		options = append(options, key)
		return nil
	})
	return options
}

// ScoreLevels returns how many levels a score question has.
func (q DecisionQuestion) ScoreLevels() int {
	if q.Type != "score" {
		return 0
	}
	var levels []json.RawMessage
	if err := json.Unmarshal(q.Criteria, &levels); err != nil {
		return 0
	}
	return len(levels)
}

// EntrypointNames returns every file the spec owns: one per question, plus the
// state file.
func (s DecisionSpec) EntrypointNames() []string {
	names := make([]string, 0, len(s.Questions)+1)
	for _, q := range s.Questions {
		names = append(names, q.ID)
	}
	if s.StateFile != "" {
		names = append(names, s.StateFile)
	}
	return names
}

// orderedObject walks a JSON object's members in document order.
func orderedObject(raw json.RawMessage, visit func(key string, value json.RawMessage) error) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected a JSON object")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected an object key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		if err := visit(key, value); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

// fieldDirective matches {{@field path}}, which renders to `path` — TypeSafe's
// reference syntax for a field of the request state. Mirrors the backend's
// FIELD_DIRECTIVE.
var fieldDirective = regexp.MustCompile(`\{\{\s*@field\s+([^{}]*?)\s*\}\}`)

// ResolveFieldDirectives replaces every {{@field path}} with `path`.
func ResolveFieldDirectives(content string) string {
	if !strings.Contains(content, "@field") {
		return content
	}
	return fieldDirective.ReplaceAllString(content, "`$1`")
}

// wholeValueTag matches a value that is exactly one Mustache variable tag.
// In a YAML file such a value is replaced by the raw input value rather than
// its string form. Must stay identical to the backend's WHOLE_VALUE_PATTERN
// and the regex emitted into generated code.
var wholeValueTag = regexp.MustCompile(`^\s*(?:\{\{\{\s*([^{}\s][^{}]*?)\s*\}\}\}|\{\{\s*&?\s*([^{}#^/!>=@\s][^{}]*?)\s*\}\})\s*$`)

// WholeValuePattern is the source of wholeValueTag, for emitting into
// generated TypeScript and Python.
const WholeValuePattern = `^\s*(?:\{\{\{\s*([^{}\s][^{}]*?)\s*\}\}\}|\{\{\s*&?\s*([^{}#^/!>=@\s][^{}]*?)\s*\}\})\s*$`

// WholeValueVariable returns the variable a value substitutes whole, or "".
func WholeValueVariable(template string) string {
	m := wholeValueTag.FindStringSubmatch(template)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(m[2])
}
