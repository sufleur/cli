package resolver

import (
	"fmt"
	"strings"
)

// PromptDiff represents a change between the existing lockfile and the resolved state.
type PromptDiff struct {
	Name           string
	OldVersion     string
	NewVersion     string
	OldConstraint  string
	NewConstraint  string
}

func (d PromptDiff) String() string {
	if d.OldVersion == "" {
		return fmt.Sprintf("  + %s@%s (new)", d.Name, d.NewVersion)
	}
	if d.NewVersion == "" {
		return fmt.Sprintf("  - %s@%s (removed)", d.Name, d.OldVersion)
	}
	return fmt.Sprintf("  ~ %s: %s → %s", d.Name, d.OldVersion, d.NewVersion)
}

// FrozenError indicates that --frozen was set but the resolved lockfile differs.
type FrozenError struct {
	Diffs []PromptDiff
}

func (e *FrozenError) Error() string {
	var b strings.Builder
	b.WriteString("lockfile is out of date (--frozen mode):\n")
	for _, d := range e.Diffs {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}
