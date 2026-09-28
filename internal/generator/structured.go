package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FormatYAML is PromptFile.format for templated structured data: the file is
// YAML whose string values are Mustache templates, rendered one by one after
// parsing ("parse first, then render"). Only question and state files of
// SYSTEM_ONE prompts use it.
const FormatYAML = "YAML"

// TreeKind is the node type of a parsed YAML template.
type TreeKind int

const (
	TreeString  TreeKind = iota // a Mustache template
	TreeLiteral                 // number, boolean or null, passed through
	TreeList
	TreeMap
)

// TreeNode is a parsed YAML template. Mapping keys keep document order.
type TreeNode struct {
	Kind    TreeKind
	Str     string          // TreeString
	Literal json.RawMessage // TreeLiteral: JSON-encoded value
	Items   []*TreeNode     // TreeList
	Keys    []string        // TreeMap, in document order
	Values  []*TreeNode     // TreeMap, parallel to Keys
}

// ParseStructured parses a YAML-format file. It applies the backend's rules:
// keys are fixed scalars, unquoted Mustache tags (which YAML reads as flow
// mappings) are rejected with a hint, and non-finite numbers are refused.
func ParseStructured(content string) (*TreeNode, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return &TreeNode{Kind: TreeLiteral, Literal: json.RawMessage("null")}, nil
	}
	return fromYAML(doc.Content[0], "")
}

func fromYAML(n *yaml.Node, path string) (*TreeNode, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return fromYAML(n.Alias, path)
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return &TreeNode{Kind: TreeLiteral, Literal: json.RawMessage("null")}, nil
		}
		return fromYAML(n.Content[0], path)
	case yaml.SequenceNode:
		out := &TreeNode{Kind: TreeList}
		for i, item := range n.Content {
			child, err := fromYAML(item, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out.Items = append(out.Items, child)
		}
		return out, nil
	case yaml.MappingNode:
		out := &TreeNode{Kind: TreeMap}
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode, valueNode := n.Content[i], n.Content[i+1]
			if keyNode.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("invalid YAML: Mustache tags must be quoted in YAML, e.g. ticket: \"{{ticket}}\"")
			}
			key := keyNode.Value
			for _, existing := range out.Keys {
				if existing == key {
					return nil, fmt.Errorf("invalid YAML: duplicate key %q", key)
				}
			}
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			child, err := fromYAML(valueNode, childPath)
			if err != nil {
				return nil, err
			}
			out.Keys = append(out.Keys, key)
			out.Values = append(out.Values, child)
		}
		return out, nil
	case yaml.ScalarNode:
		return scalarNode(n, path)
	}
	return nil, fmt.Errorf("unsupported YAML node at %s", displayPath(path))
}

func scalarNode(n *yaml.Node, path string) (*TreeNode, error) {
	switch n.Tag {
	case "!!null":
		return &TreeNode{Kind: TreeLiteral, Literal: json.RawMessage("null")}, nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return nil, err
		}
		return &TreeNode{Kind: TreeLiteral, Literal: json.RawMessage(strconv.FormatBool(b))}, nil
	case "!!int", "!!float":
		var f float64
		if err := n.Decode(&f); err != nil {
			return nil, err
		}
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("unsupported non-finite number at %s", displayPath(path))
		}
		raw, _ := json.Marshal(f)
		return &TreeNode{Kind: TreeLiteral, Literal: raw}, nil
	default:
		return &TreeNode{Kind: TreeString, Str: n.Value}, nil
	}
}

func displayPath(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

// Templates returns every string value, in document order.
func (n *TreeNode) Templates() []string {
	switch n.Kind {
	case TreeString:
		return []string{n.Str}
	case TreeList:
		var out []string
		for _, item := range n.Items {
			out = append(out, item.Templates()...)
		}
		return out
	case TreeMap:
		var out []string
		for _, v := range n.Values {
			out = append(out, v.Templates()...)
		}
		return out
	}
	return nil
}

// Map applies fn to every string value, returning a new tree.
func (n *TreeNode) Map(fn func(string) string) *TreeNode {
	switch n.Kind {
	case TreeString:
		return &TreeNode{Kind: TreeString, Str: fn(n.Str)}
	case TreeList:
		out := &TreeNode{Kind: TreeList}
		for _, item := range n.Items {
			out.Items = append(out.Items, item.Map(fn))
		}
		return out
	case TreeMap:
		out := &TreeNode{Kind: TreeMap, Keys: append([]string(nil), n.Keys...)}
		for _, v := range n.Values {
			out.Values = append(out.Values, v.Map(fn))
		}
		return out
	}
	return n
}

// MarshalJSON emits the tree as JSON with mapping keys in document order.
func (n *TreeNode) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	if err := n.writeJSON(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (n *TreeNode) writeJSON(b *bytes.Buffer) error {
	switch n.Kind {
	case TreeString:
		raw, err := marshalUnescaped(n.Str)
		if err != nil {
			return err
		}
		b.Write(raw)
	case TreeLiteral:
		b.Write(n.Literal)
	case TreeList:
		b.WriteByte('[')
		for i, item := range n.Items {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := item.writeJSON(b); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case TreeMap:
		b.WriteByte('{')
		for i, key := range n.Keys {
			if i > 0 {
				b.WriteByte(',')
			}
			raw, err := marshalUnescaped(key)
			if err != nil {
				return err
			}
			b.Write(raw)
			b.WriteByte(':')
			if err := n.Values[i].writeJSON(b); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	}
	return nil
}

// LookupView resolves a dotted Mustache name against a view. "." is the view.
func LookupView(view any, name string) (any, bool) {
	if name == "." {
		return view, true
	}
	current := view
	for _, key := range strings.Split(name, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// RenderStructured renders a parsed YAML template: whole-value variables become
// the input value itself (null when missing); every other string is rendered
// with renderString. The result keeps mapping order.
func RenderStructured(n *TreeNode, view map[string]any, renderString func(string) (string, error)) (*TreeNode, error) {
	switch n.Kind {
	case TreeString:
		if variable := WholeValueVariable(n.Str); variable != "" {
			value, ok := LookupView(view, variable)
			if !ok {
				value = nil
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return &TreeNode{Kind: TreeLiteral, Literal: raw}, nil
		}
		rendered, err := renderString(n.Str)
		if err != nil {
			return nil, err
		}
		return &TreeNode{Kind: TreeString, Str: rendered}, nil
	case TreeList:
		out := &TreeNode{Kind: TreeList}
		for _, item := range n.Items {
			child, err := RenderStructured(item, view, renderString)
			if err != nil {
				return nil, err
			}
			out.Items = append(out.Items, child)
		}
		return out, nil
	case TreeMap:
		out := &TreeNode{Kind: TreeMap, Keys: append([]string(nil), n.Keys...)}
		for _, v := range n.Values {
			child, err := RenderStructured(v, view, renderString)
			if err != nil {
				return nil, err
			}
			out.Values = append(out.Values, child)
		}
		return out, nil
	}
	return n, nil
}

// marshalUnescaped encodes a string without HTML escaping, so templates like
// {{> partial}} stay readable wherever the tree is emitted.
func marshalUnescaped(s string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// JSONToYAML converts JSON to YAML keeping object key order, so an
// order-sensitive document (a decision spec) round-trips through a YAML file
// without its questions or options being re-sorted.
func JSONToYAML(raw []byte) ([]byte, error) {
	node, err := jsonToYAMLNode(json.RawMessage(raw))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jsonToYAMLNode(raw json.RawMessage) (*yaml.Node, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	switch trimmed[0] {
	case '{':
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		err := orderedObject(raw, func(key string, value json.RawMessage) error {
			child, err := jsonToYAMLNode(value)
			if err != nil {
				return err
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
			return nil
		})
		return node, err
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range items {
			child, err := jsonToYAMLNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	default:
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		node := &yaml.Node{}
		if err := node.Encode(value); err != nil {
			return nil, err
		}
		return node, nil
	}
}
