package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// KindSystemOne is the Prompt.kind value of decision prompts: typed
// noul / choice / score question templates for a System-One model (TypeSafe
// Jev), instead of chat templates for a text-generating LLM.
const KindSystemOne = "SYSTEM_ONE"

// IsDecision reports whether the prompt is a SYSTEM_ONE decision prompt.
func (p PromptData) IsDecision() bool {
	return p.Kind == KindSystemOne && p.DecisionSpec != nil
}

// DecisionSpec is a SYSTEM_ONE version's question templates. Each question id
// is also the name of the entrypoint file holding its instructions; string
// values in criteria are Mustache templates rendered with the question's
// inputs. Questions keep the order they were authored in — the backend stores
// the spec as order-preserving JSON, and this type decodes it without going
// through a Go map, so generated code and dumps list questions (and choice
// options) in author order.
type DecisionSpec struct {
	Questions []DecisionQuestion
}

// DecisionQuestion is one question template. Criteria and OptionCriteria are
// kept as raw JSON so option order and structured (object/array) descriptions
// survive untouched. OptionCriteria is nil when absent; a present-but-null
// value (an open choice whose added options have no description) is "null".
type DecisionQuestion struct {
	ID             string
	Type           string // "noul" | "choice" | "score"
	Criteria       json.RawMessage
	OptionCriteria json.RawMessage
}

// IsOpenChoice reports whether callers may add options to this question.
func (q DecisionQuestion) IsOpenChoice() bool {
	return q.Type == "choice" && q.OptionCriteria != nil
}

// UnmarshalJSON decodes {questions: {id: {type, criteria?, optionCriteria?}}}
// preserving the order of the question keys. A legacy stateFile key is ignored.
func (s *DecisionSpec) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("decision spec: %w", err)
	}
	*s = DecisionSpec{}
	raw, ok := top["questions"]
	if !ok {
		return nil
	}
	return orderedObject(raw, func(id string, value json.RawMessage) error {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(value, &fields); err != nil {
			return fmt.Errorf("decision spec question %q: %w", id, err)
		}
		var typ string
		if err := json.Unmarshal(fields["type"], &typ); err != nil {
			return fmt.Errorf("decision spec question %q type: %w", id, err)
		}
		criteria := fields["criteria"]
		if bytes.Equal(bytes.TrimSpace(criteria), []byte("null")) {
			criteria = nil
		}
		var optionCriteria json.RawMessage
		if raw, ok := fields["optionCriteria"]; ok {
			optionCriteria = append(json.RawMessage(nil), bytes.TrimSpace(raw)...)
		}
		s.Questions = append(s.Questions, DecisionQuestion{
			ID: id, Type: typ, Criteria: criteria, OptionCriteria: optionCriteria,
		})
		return nil
	})
}

// MarshalJSON writes the spec back in the wire shape, in question order.
// Deterministic output keeps cache files and lockfile integrity hashes stable.
func (s DecisionSpec) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"questions":{`)
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
		if q.OptionCriteria != nil {
			var compact bytes.Buffer
			if err := json.Compact(&compact, q.OptionCriteria); err != nil {
				return nil, err
			}
			b.WriteString(`,"optionCriteria":`)
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

// EntrypointNames returns every file the spec owns: one per question.
func (s DecisionSpec) EntrypointNames() []string {
	names := make([]string, 0, len(s.Questions))
	for _, q := range s.Questions {
		names = append(names, q.ID)
	}
	return names
}

// Question returns the question with the given id.
func (s DecisionSpec) Question(id string) (DecisionQuestion, bool) {
	for _, q := range s.Questions {
		if q.ID == id {
			return q, true
		}
	}
	return DecisionQuestion{}, false
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
