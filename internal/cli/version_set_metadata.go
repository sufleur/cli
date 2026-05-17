package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionSetMetadataCmd = &cobra.Command{
	Use:           "set-metadata @workspace/name@version",
	Short:         "Set or sync metadata keys on a version",
	Long: `Two modes:

Flag mode (additive patch): pass one or more of
  --string KEY=VALUE
  --integer KEY=VALUE
  --float KEY=VALUE
  --boolean KEY=VALUE
Existing keys not mentioned are left alone.

File mode (full replace): pass --from-file PATH to a YAML file with flat
key-value pairs. Types are inferred from each YAML value (string,
integer, float, boolean). Keys present in the file are set; keys present
on the version but absent from the file are deleted.

The two modes are mutually exclusive.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
		}

		fromFile, _ := cmd.Flags().GetString("from-file")
		strings_, _ := cmd.Flags().GetStringArray("string")
		integers, _ := cmd.Flags().GetStringArray("integer")
		floats, _ := cmd.Flags().GetStringArray("float")
		booleans, _ := cmd.Flags().GetStringArray("boolean")
		hasFlagValues := len(strings_)+len(integers)+len(floats)+len(booleans) > 0

		if fromFile != "" && hasFlagValues {
			return fmt.Errorf("--from-file is mutually exclusive with --string/--integer/--float/--boolean")
		}
		if fromFile == "" && !hasFlagValues {
			return fmt.Errorf("nothing to set: pass --string/--integer/--float/--boolean or --from-file")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		var finalVersion *userapi.PromptVersion
		var summary metadataSummary
		if fromFile != "" {
			finalVersion, summary, err = syncMetadataFromFile(cmd.Context(), client, ref, fromFile)
		} else {
			finalVersion, summary, err = applyMetadataFlags(cmd.Context(), client, ref, strings_, integers, floats, booleans)
		}
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, finalVersion)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set %d, deleted %d on @%s/%s@%s\n",
			summary.set, summary.deleted, ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	versionSetMetadataCmd.Flags().String("from-file", "", "Path to a YAML file with flat scalar key-value pairs (full-replace mode)")
	versionSetMetadataCmd.Flags().StringArray("string", nil, "Set a string metadata key (repeatable, K=V)")
	versionSetMetadataCmd.Flags().StringArray("integer", nil, "Set an integer metadata key (repeatable, K=V)")
	versionSetMetadataCmd.Flags().StringArray("float", nil, "Set a float metadata key (repeatable, K=V)")
	versionSetMetadataCmd.Flags().StringArray("boolean", nil, "Set a boolean metadata key (repeatable, K=V)")
}

type metadataSummary struct {
	set     int
	deleted int
}

func applyMetadataFlags(ctx context.Context, client *userapi.Client, ref promptref.PromptRef,
	strings_, integers, floats, booleans []string) (*userapi.PromptVersion, metadataSummary, error) {
	var summary metadataSummary
	var last *userapi.PromptVersion

	for _, kv := range strings_ {
		k, v, err := splitKV(kv)
		if err != nil {
			return nil, summary, err
		}
		out, err := client.SetPromptVersionStringMetadata(ctx, ref.Workspace, ref.Name, ref.Version, k, v)
		if err != nil {
			return nil, summary, fmt.Errorf("setting %s: %w", k, err)
		}
		last = out
		summary.set++
	}
	for _, kv := range integers {
		k, v, err := splitKV(kv)
		if err != nil {
			return nil, summary, err
		}
		n, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return nil, summary, fmt.Errorf("integer %s=%s: %w", k, v, perr)
		}
		out, err := client.SetPromptVersionIntegerMetadata(ctx, ref.Workspace, ref.Name, ref.Version, k, n)
		if err != nil {
			return nil, summary, fmt.Errorf("setting %s: %w", k, err)
		}
		last = out
		summary.set++
	}
	for _, kv := range floats {
		k, v, err := splitKV(kv)
		if err != nil {
			return nil, summary, err
		}
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			return nil, summary, fmt.Errorf("float %s=%s: %w", k, v, perr)
		}
		out, err := client.SetPromptVersionFloatMetadata(ctx, ref.Workspace, ref.Name, ref.Version, k, f)
		if err != nil {
			return nil, summary, fmt.Errorf("setting %s: %w", k, err)
		}
		last = out
		summary.set++
	}
	for _, kv := range booleans {
		k, v, err := splitKV(kv)
		if err != nil {
			return nil, summary, err
		}
		b, perr := strconv.ParseBool(v)
		if perr != nil {
			return nil, summary, fmt.Errorf("boolean %s=%s: %w", k, v, perr)
		}
		out, err := client.SetPromptVersionBooleanMetadata(ctx, ref.Workspace, ref.Name, ref.Version, k, b)
		if err != nil {
			return nil, summary, fmt.Errorf("setting %s: %w", k, err)
		}
		last = out
		summary.set++
	}
	if last == nil {
		// Should not happen given the earlier nothing-to-set guard.
		return nil, summary, fmt.Errorf("nothing to set")
	}
	return last, summary, nil
}

func syncMetadataFromFile(ctx context.Context, client *userapi.Client, ref promptref.PromptRef, path string) (*userapi.PromptVersion, metadataSummary, error) {
	var summary metadataSummary

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, summary, fmt.Errorf("reading %s: %w", path, err)
	}
	desired := map[string]any{}
	if err := yaml.Unmarshal(data, &desired); err != nil {
		return nil, summary, fmt.Errorf("parsing %s: %w", path, err)
	}
	for k, v := range desired {
		if v == nil {
			return nil, summary, fmt.Errorf("metadata key %q has null value; remove it from the file to delete it", k)
		}
		if _, isMap := v.(map[string]any); isMap {
			return nil, summary, fmt.Errorf("metadata key %q is a nested object; only scalar values are supported", k)
		}
		if _, isSlice := v.([]any); isSlice {
			return nil, summary, fmt.Errorf("metadata key %q is an array; only scalar values are supported", k)
		}
	}

	current, err := client.GetPromptVersion(ctx, ref.Workspace, ref.Name, ref.Version)
	if err != nil {
		return nil, summary, err
	}

	// Delete keys present in current but not in desired.
	toDelete := make([]string, 0)
	for k := range current.Metadata {
		if _, keep := desired[k]; !keep {
			toDelete = append(toDelete, k)
		}
	}
	sort.Strings(toDelete)
	var last *userapi.PromptVersion = current
	for _, k := range toDelete {
		out, err := client.DeletePromptVersionMetadata(ctx, ref.Workspace, ref.Name, ref.Version, k)
		if err != nil {
			return nil, summary, fmt.Errorf("deleting %s: %w", k, err)
		}
		last = out
		summary.deleted++
	}

	// Set every key in desired. Iterate in deterministic order for reproducibility.
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out, err := setMetadataByGoType(ctx, client, ref, k, desired[k])
		if err != nil {
			return nil, summary, fmt.Errorf("setting %s: %w", k, err)
		}
		last = out
		summary.set++
	}
	return last, summary, nil
}

func setMetadataByGoType(ctx context.Context, client *userapi.Client, ref promptref.PromptRef, key string, value any) (*userapi.PromptVersion, error) {
	switch v := value.(type) {
	case string:
		return client.SetPromptVersionStringMetadata(ctx, ref.Workspace, ref.Name, ref.Version, key, v)
	case bool:
		return client.SetPromptVersionBooleanMetadata(ctx, ref.Workspace, ref.Name, ref.Version, key, v)
	case int:
		return client.SetPromptVersionIntegerMetadata(ctx, ref.Workspace, ref.Name, ref.Version, key, int64(v))
	case int64:
		return client.SetPromptVersionIntegerMetadata(ctx, ref.Workspace, ref.Name, ref.Version, key, v)
	case uint64:
		return client.SetPromptVersionIntegerMetadata(ctx, ref.Workspace, ref.Name, ref.Version, key, int64(v))
	case float64:
		return client.SetPromptVersionFloatMetadata(ctx, ref.Workspace, ref.Name, ref.Version, key, v)
	default:
		return nil, fmt.Errorf("unsupported value type %T for key %q (only string/integer/float/boolean)", value, key)
	}
}

func splitKV(s string) (string, string, error) {
	i := strings.IndexByte(s, '=')
	if i <= 0 {
		return "", "", fmt.Errorf("expected KEY=VALUE, got %q", s)
	}
	return s[:i], s[i+1:], nil
}
