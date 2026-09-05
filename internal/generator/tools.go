package generator

import (
	"fmt"
	"sort"
	"strings"
)

// ToPascalCase converts a package ref or file name to a PascalCase identifier,
// treating "-", "_", "@" and "/" as word separators.
//
//	"@acme/web-search" -> "AcmeWebSearch"
func ToPascalCase(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if r == '-' || r == '_' || r == '@' || r == '/' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(toUpperASCII(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func toUpperASCII(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// ToolKey identifies one distinct tool contract across the whole install set.
// Two prompts pinning the same tool version share a key and therefore one set
// of generated types, whatever wire names they each use.
func ToolKey(p ToolPin) string { return p.Ref + "@" + p.Version }

// ToolPlan is the emit plan for tool contracts: which distinct contracts to
// emit, and what to call each one. Both generators build it from the same
// prompt set so their type names agree.
type ToolPlan struct {
	// Keys are ToolKey values in emit order: sorted by ref, then version, so
	// output never depends on map iteration or prompt ordering.
	Keys []string
	// Pins is one representative pin per key. The Alias on it is arbitrary —
	// it belongs to whichever prompt was seen first — and must not be used.
	Pins map[string]ToolPin
	// BaseNames maps a key to its generated identifier stem, e.g.
	// "AcmeWebSearchTool". Derived names append Input/Output/InputSchema.
	BaseNames map[string]string
	// Renamed lists refs that were version-suffixed because the install set
	// pins more than one of their versions, for a note to the user.
	Renamed []string
}

// Empty reports whether no prompt in the set pins anything.
func (p ToolPlan) Empty() bool { return len(p.Keys) == 0 }

// PlanTools validates the pinned contracts across a prompt set and decides what
// each generated type is called.
//
// Naming rule: a ref whose install set resolves to a single version gets the
// bare name "AcmeWebSearchTool". A ref pinned at two or more versions has every
// one of its versions suffixed — "AcmeWebSearchToolV1_2_0" and
// "...ToolV2_0_0", never a bare/suffixed mix. Suffixing only the newcomer would
// make an existing name depend on what else happens to be installed; this way
// the rule is "a ref's names are all bare or all suffixed", introducing a
// second version renames both at once, and the compiler says so immediately.
func PlanTools(prompts []PromptData) (ToolPlan, error) {
	plan := ToolPlan{Pins: map[string]ToolPin{}, BaseNames: map[string]string{}}

	versionsByRef := map[string]map[string]bool{}
	for _, pd := range prompts {
		seenAlias := map[string]bool{}
		for _, pin := range pd.Tools {
			if !AliasRe.MatchString(pin.Alias) {
				return ToolPlan{}, fmt.Errorf(
					"prompt %q pins tool %q under the wire name %q, which providers reject (must match %s)",
					displayRef(pd), pin.Ref, pin.Alias, AliasRe.String())
			}
			if seenAlias[pin.Alias] {
				return ToolPlan{}, fmt.Errorf(
					"prompt %q pins two tools under the wire name %q; the model could not tell them apart",
					displayRef(pd), pin.Alias)
			}
			seenAlias[pin.Alias] = true

			key := ToolKey(pin)
			if _, ok := plan.Pins[key]; !ok {
				plan.Pins[key] = pin
				plan.Keys = append(plan.Keys, key)
			}
			if versionsByRef[pin.Ref] == nil {
				versionsByRef[pin.Ref] = map[string]bool{}
			}
			versionsByRef[pin.Ref][pin.Version] = true
		}
	}

	if len(plan.Keys) == 0 {
		return ToolPlan{}, nil
	}

	sort.Slice(plan.Keys, func(i, j int) bool {
		a, b := plan.Pins[plan.Keys[i]], plan.Pins[plan.Keys[j]]
		if a.Ref != b.Ref {
			return a.Ref < b.Ref
		}
		return a.Version < b.Version
	})

	for _, key := range plan.Keys {
		pin := plan.Pins[key]
		base := ToPascalCase(pin.Ref) + "Tool"
		if len(versionsByRef[pin.Ref]) > 1 {
			base += versionSuffix(pin.Version)
		}
		plan.BaseNames[key] = base
	}

	for ref, versions := range versionsByRef {
		if len(versions) > 1 {
			plan.Renamed = append(plan.Renamed, ref)
		}
	}
	sort.Strings(plan.Renamed)

	return plan, nil
}

// versionSuffix turns a version into an identifier fragment: "1.2.0" -> "V1_2_0".
// A draft has no semver to encode, and a project can hold at most one draft per
// ref, so "Draft" is unambiguous within the group.
func versionSuffix(version string) string {
	if version == DraftVersion {
		return "Draft"
	}
	return "V" + strings.NewReplacer(".", "_", "-", "_", "+", "_").Replace(version)
}

// DraftVersion is the version string the registry uses for an unpublished version.
const DraftVersion = "draft"

// DraftToolAliases returns the wire names of pins on unpublished tool versions,
// in alias order. A draft prompt may pin a draft tool: the contract is still
// mutable, so generated code can go stale without the version changing.
func DraftToolAliases(pd PromptData) []string {
	var aliases []string
	for _, pin := range pd.Tools {
		if pin.Status == "DRAFT" {
			aliases = append(aliases, pin.Alias)
		}
	}
	return aliases
}

func displayRef(pd PromptData) string {
	if pd.Ref != "" {
		return pd.Ref
	}
	return pd.Name
}
