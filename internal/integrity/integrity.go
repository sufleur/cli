package integrity

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/WTomas/sufleur-cli/internal/generator"
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
}

type canonicalFile struct {
	Name         string                 `json:"name"`
	Content      string                 `json:"content"`
	IsEntrypoint bool                   `json:"isEntrypoint"`
	InputSchema  map[string]interface{} `json:"inputSchema,omitempty"`
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
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	cd := canonicalData{
		Name:        pd.Name,
		Version:     pd.Version,
		Description: pd.Description,
		Files:       files,
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
