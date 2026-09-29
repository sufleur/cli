package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Choice option limits, mirroring the backend.
const (
	ChoiceMinOptions      = 2
	ChoiceMaxOptions      = 255
	ChoiceOptionMaxLength = 255
)

// QuestionInstructions is a question's instructions file: plain Mustache text,
// or a parsed YAML template.
type QuestionInstructions struct {
	YAML     bool
	Template string
	Tree     *TreeNode
}

// DecisionOption is one option added to an open choice at request time.
type DecisionOption struct {
	Key    string
	Inputs map[string]any
}

// RenderString renders one Mustache string (partials and escaping are the
// caller's concern).
type RenderString func(template string, view map[string]any) (string, error)

// RenderDecisionQuestion renders one question into {type, instructions,
// criteria} JSON. It is the Go port of the backend's renderDecisionQuestion;
// both are checked against testdata/decision-render-cases.json.
func RenderDecisionQuestion(
	q DecisionQuestion,
	instructions QuestionInstructions,
	inputs map[string]any,
	options []DecisionOption,
	render RenderString,
) (json.RawMessage, error) {
	if inputs == nil {
		inputs = map[string]any{}
	}
	var instructionsJSON []byte
	if instructions.YAML {
		tree, err := RenderStructured(instructions.Tree, inputs, func(t string) (string, error) {
			return render(t, inputs)
		})
		if err != nil {
			return nil, err
		}
		if instructionsJSON, err = tree.MarshalJSON(); err != nil {
			return nil, err
		}
	} else {
		text, err := render(instructions.Template, inputs)
		if err != nil {
			return nil, err
		}
		if instructionsJSON, err = marshalUnescaped(text); err != nil {
			return nil, err
		}
	}

	var b bytes.Buffer
	typ, _ := marshalUnescaped(q.Type)
	b.WriteString(`{"type":`)
	b.Write(typ)
	b.WriteString(`,"instructions":`)
	b.Write(instructionsJSON)

	if q.Type != "choice" {
		if len(options) > 0 {
			return nil, fmt.Errorf("%q is a %s question: it takes no options", q.ID, q.Type)
		}
		if len(q.Criteria) > 0 {
			criteria, err := renderJSONStrings(q.Criteria, inputs, render)
			if err != nil {
				return nil, err
			}
			b.WriteString(`,"criteria":`)
			b.Write(criteria)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	}

	if len(options) > 0 && !q.IsOpenChoice() {
		return nil, fmt.Errorf("%q has a fixed set of options: add optionCriteria to let callers add options", q.ID)
	}
	var keys []string
	var values [][]byte
	if len(q.Criteria) > 0 {
		err := orderedObject(q.Criteria, func(key string, value json.RawMessage) error {
			rendered, err := renderJSONStrings(value, inputs, render)
			if err != nil {
				return err
			}
			keys = append(keys, key)
			values = append(values, rendered)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	fixed := map[string]bool{}
	for _, key := range keys {
		fixed[key] = true
	}
	for _, option := range options {
		if strings.TrimSpace(option.Key) == "" || len(option.Key) > ChoiceOptionMaxLength {
			return nil, fmt.Errorf("%q: option keys must be non-blank and at most %d characters", q.ID, ChoiceOptionMaxLength)
		}
		if fixed[option.Key] {
			return nil, fmt.Errorf("%q: option %q is already one of the fixed options", q.ID, option.Key)
		}
		inputs := option.Inputs
		if inputs == nil {
			inputs = map[string]any{}
		}
		rendered, err := renderJSONStrings(q.OptionCriteria, inputs, render)
		if err != nil {
			return nil, err
		}
		fixed[option.Key] = true
		keys = append(keys, option.Key)
		values = append(values, rendered)
	}
	if len(keys) < ChoiceMinOptions || len(keys) > ChoiceMaxOptions {
		return nil, fmt.Errorf("%q: a choice needs between %d and %d options (this one has %d)", q.ID, ChoiceMinOptions, ChoiceMaxOptions, len(keys))
	}
	b.WriteString(`,"criteria":{`)
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := marshalUnescaped(key)
		b.Write(k)
		b.WriteByte(':')
		b.Write(values[i])
	}
	b.WriteString("}}")
	return b.Bytes(), nil
}

// renderJSONStrings renders every string value of a JSON document with
// Mustache, keeping keys, key order and non-string values unchanged.
func renderJSONStrings(raw json.RawMessage, view map[string]any, render RenderString) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []byte("null"), nil
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		out, err := render(s, view)
		if err != nil {
			return nil, err
		}
		return marshalUnescaped(out)
	case '{':
		var b bytes.Buffer
		b.WriteByte('{')
		first := true
		err := orderedObject(trimmed, func(key string, value json.RawMessage) error {
			rendered, err := renderJSONStrings(value, view, render)
			if err != nil {
				return err
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			k, _ := marshalUnescaped(key)
			b.Write(k)
			b.WriteByte(':')
			b.Write(rendered)
			return nil
		})
		if err != nil {
			return nil, err
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		var b bytes.Buffer
		b.WriteByte('[')
		for i, item := range items {
			if i > 0 {
				b.WriteByte(',')
			}
			rendered, err := renderJSONStrings(item, view, render)
			if err != nil {
				return nil, err
			}
			b.Write(rendered)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}

// OrderedOptions decodes a JSON object of option key → option inputs,
// keeping the keys in document order.
func OrderedOptions(raw []byte) ([]DecisionOption, error) {
	var options []DecisionOption
	err := orderedObject(raw, func(key string, value json.RawMessage) error {
		var inputs map[string]any
		if err := json.Unmarshal(value, &inputs); err != nil {
			return fmt.Errorf("option %q: inputs must be an object: %w", key, err)
		}
		options = append(options, DecisionOption{Key: key, Inputs: inputs})
		return nil
	})
	return options, err
}
