package integrity

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/sufleur/cli/internal/generator"
)

// IntegrityError indicates a mismatch between expected and actual content hashes.
type IntegrityError struct {
	PromptName string
	Expected   string
	Actual     string
}

func (e *IntegrityError) Error() string {
	return fmt.Sprintf("integrity mismatch for %q: expected %s, got %s", e.PromptName, e.Expected, e.Actual)
}

// canonicalData is the serializable form with sorted files.
type canonicalData struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Files       []canonicalFile `json:"files"`
	// Tools is omitted entirely for a prompt that pins none, so hashes computed
	// before tool support stay valid — no forced refetch or --frozen failure on
	// upgrade. Never drop the omitempty.
	Tools []canonicalTool `json:"tools,omitempty"`
	// DecisionSpec is set only for decision (SYSTEM_ONE) prompts, so LLM
	// prompt hashes are unchanged. Never drop the omitempty.
	DecisionSpec json.RawMessage `json:"decisionSpec,omitempty"`
}

type canonicalFile struct {
	Name         string                 `json:"name"`
	Content      string                 `json:"content"`
	IsEntrypoint bool                   `json:"isEntrypoint"`
	InputSchema  map[string]interface{} `json:"inputSchema,omitempty"`
	// Format and OptionInputSchema only ever appear on decision-prompt files,
	// so LLM prompt hashes are unchanged. Never drop the omitempty.
	Format            string                 `json:"format,omitempty"`
	OptionInputSchema map[string]interface{} `json:"optionInputSchema,omitempty"`
}

// canonicalTool is a pinned tool contract as it contributes to the hash.
//
// The whole contract is hashed, not just the tool's identity: the pinned text
// and schemas are what the generated code is built from, so a cached copy that
// has been altered should fail verification exactly as an altered template
// does. Metadata is excluded, matching the prompt's own metadata exclusion.
type canonicalTool struct {
	Alias            string                 `json:"alias"`
	Ref              string                 `json:"ref"`
	Version          string                 `json:"version"`
	Status           string                 `json:"status"`
	ModelDescription string                 `json:"modelDescription"`
	InputSchema      map[string]interface{} `json:"inputSchema,omitempty"`
	OutputSchema     map[string]interface{} `json:"outputSchema,omitempty"`
}

// Compute returns a deterministic SHA-256 digest of the prompt data.
// Files are sorted by name to ensure order independence.
// The returned string has the form "sha256-<hex>".
func Compute(pd *generator.PromptData) string {
	files := make([]canonicalFile, len(pd.Files))
	for i, f := range pd.Files {
		files[i] = canonicalFile{
			Name:         f.Name,
			Content:      f.Content,
			IsEntrypoint: f.IsEntrypoint,
			InputSchema:  f.InputSchema,

			Format:            f.Format,
			OptionInputSchema: f.OptionInputSchema,
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	// Left nil when the prompt pins nothing, so the marshalled bytes are
	// identical to what this produced before tools existed.
	var tools []canonicalTool
	if len(pd.Tools) > 0 {
		tools = make([]canonicalTool, len(pd.Tools))
		for i, p := range pd.Tools {
			tools[i] = canonicalTool{
				Alias:            p.Alias,
				Ref:              p.Ref,
				Version:          p.Version,
				Status:           p.Status,
				ModelDescription: p.ModelDescription,
				InputSchema:      p.InputSchema,
				OutputSchema:     p.OutputSchema,
			}
		}
		// Aliases are unique within a prompt version, so this is a total order.
		sort.Slice(tools, func(i, j int) bool { return tools[i].Alias < tools[j].Alias })
	}

	cd := canonicalData{
		Name:        pd.Name,
		Version:     pd.Version,
		Description: pd.Description,
		Files:       files,
		Tools:       tools,
	}
	if pd.DecisionSpec != nil {
		if spec, err := pd.DecisionSpec.MarshalJSON(); err == nil {
			cd.DecisionSpec = spec
		}
	}

	data, _ := json.Marshal(cd) // struct is always marshalable
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256-%x", h)
}

// Verify recomputes the integrity hash and compares it to expected.
func Verify(pd *generator.PromptData, expected string) error {
	actual := Compute(pd)
	if actual != expected {
		return &IntegrityError{
			PromptName: pd.Name,
			Expected:   expected,
			Actual:     actual,
		}
	}
	return nil
}
