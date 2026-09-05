package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var toolVersionSetMetadataCmd = &cobra.Command{
	Use:   "set-metadata @workspace/name@draft [--from-file meta.yaml | --string K=V ...]",
	Short: "Set metadata on a draft tool version",
	Long: `Sets the free-form metadata object on a draft version.

  --from-file PATH   replace the whole object from a JSON file (- for stdin)
  --string K=V       set one string key      (repeatable)
  --integer K=V      set one integer key     (repeatable)
  --float K=V        set one float key       (repeatable)
  --boolean K=V      set one boolean key     (repeatable)
  --delete K         remove one key          (repeatable)

The two modes are mutually exclusive.

Unlike a prompt version's metadata, a tool version's is a plain JSON object with
no per-key mutation on the server. The typed flags are therefore a
read-modify-write: the current object is fetched, patched, and written back
whole. Two concurrent edits can lose one another.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}

		fromFile, _ := cmd.Flags().GetString("from-file")
		patches, err := collectMetadataPatches(cmd)
		if err != nil {
			return err
		}
		if fromFile != "" && len(patches) > 0 {
			return fmt.Errorf("--from-file and the per-key flags are mutually exclusive")
		}
		if fromFile == "" && len(patches) == 0 {
			return fmt.Errorf("nothing to set: pass --from-file or at least one key flag")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		metadata := map[string]any{}
		if fromFile != "" {
			raw, err := readFileOrStdin(cmd, fromFile)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &metadata); err != nil {
				return fmt.Errorf("parsing metadata as a JSON object: %w", err)
			}
		} else {
			// Read-modify-write: the server has no per-key mutation for this field.
			current, err := client.GetToolVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
			if err != nil {
				return mapBearer(err)
			}
			for k, v := range current.Metadata {
				metadata[k] = v
			}
			for _, p := range patches {
				if p.remove {
					delete(metadata, p.key)
					continue
				}
				metadata[p.key] = p.value
			}
		}

		v, err := client.SetToolVersionMetadata(cmd.Context(), ref.Workspace, ref.Name, ref.Version, metadata)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set %d metadata key(s) on @%s/%s@%s.\n",
			len(metadata), ref.Workspace, ref.Name, v.Version)
		return nil
	},
}

type metadataPatch struct {
	key    string
	value  any
	remove bool
}

// collectMetadataPatches parses the typed key flags in a stable order, so the
// same invocation always produces the same object.
func collectMetadataPatches(cmd *cobra.Command) ([]metadataPatch, error) {
	var patches []metadataPatch

	strs, _ := cmd.Flags().GetStringArray("string")
	for _, kv := range strs {
		k, v, err := splitMetadataKV(kv)
		if err != nil {
			return nil, err
		}
		patches = append(patches, metadataPatch{key: k, value: v})
	}

	ints, _ := cmd.Flags().GetStringArray("integer")
	for _, kv := range ints {
		k, v, err := splitMetadataKV(kv)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--integer %s: %q is not an integer", kv, v)
		}
		patches = append(patches, metadataPatch{key: k, value: n})
	}

	floats, _ := cmd.Flags().GetStringArray("float")
	for _, kv := range floats {
		k, v, err := splitMetadataKV(kv)
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("--float %s: %q is not a number", kv, v)
		}
		patches = append(patches, metadataPatch{key: k, value: f})
	}

	bools, _ := cmd.Flags().GetStringArray("boolean")
	for _, kv := range bools {
		k, v, err := splitMetadataKV(kv)
		if err != nil {
			return nil, err
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("--boolean %s: %q is not true or false", kv, v)
		}
		patches = append(patches, metadataPatch{key: k, value: b})
	}

	deletes, _ := cmd.Flags().GetStringArray("delete")
	for _, k := range deletes {
		if k == "" {
			return nil, fmt.Errorf("--delete requires a key name")
		}
		patches = append(patches, metadataPatch{key: k, remove: true})
	}

	return patches, nil
}

func splitMetadataKV(kv string) (string, string, error) {
	key, value, found := strings.Cut(kv, "=")
	if !found || key == "" {
		return "", "", fmt.Errorf("expected KEY=VALUE, got %q", kv)
	}
	return key, value, nil
}

func init() {
	toolVersionSetMetadataCmd.Flags().String("from-file", "", "Path to a JSON file replacing the whole metadata object (- for stdin)")
	toolVersionSetMetadataCmd.Flags().StringArray("string", nil, "Set a string key (repeatable, K=V)")
	toolVersionSetMetadataCmd.Flags().StringArray("integer", nil, "Set an integer key (repeatable, K=V)")
	toolVersionSetMetadataCmd.Flags().StringArray("float", nil, "Set a float key (repeatable, K=V)")
	toolVersionSetMetadataCmd.Flags().StringArray("boolean", nil, "Set a boolean key (repeatable, K=V)")
	toolVersionSetMetadataCmd.Flags().StringArray("delete", nil, "Remove a key (repeatable)")
}
