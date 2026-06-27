package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var workspaceProvidersCmd = &cobra.Command{
	Use:   "providers @workspace",
	Short: "List the AI providers configured for a workspace",
	Long: `Lists the AI provider credentials configured for a workspace (provider, the
friendly name given at setup, and the last four characters of the key). An eval
can only run against a provider that appears here.

Pass --models to also list the models available for each provider. That call
reaches the provider's live API, so it can fail for an individual provider (e.g.
an expired key) without affecting the others.

Configuring provider credentials is done in the web UI — the CLI can only list
them.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := parseWorkspaceArg(args[0])
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		creds, err := client.ListProviderCredentials(cmd.Context(), ws)
		if err != nil {
			return mapBearer(err)
		}

		withModels, _ := cmd.Flags().GetBool("models")
		asJSON, _ := cmd.Flags().GetBool("json")

		// Gather models per provider when requested, isolating per-provider failures.
		type providerEntry struct {
			userapi.ProviderCredential
			Models      []userapi.SupportedModel `json:"models,omitempty"`
			ModelsError string                   `json:"modelsError,omitempty"`
		}
		var entries []providerEntry
		if withModels {
			for _, c := range creds {
				e := providerEntry{ProviderCredential: c}
				models, mErr := client.AvailableModels(cmd.Context(), ws, c.Provider)
				if mErr != nil {
					e.ModelsError = mErr.Error()
				} else {
					e.Models = models
				}
				entries = append(entries, e)
			}
		}

		if asJSON {
			if withModels {
				return printJSON(cmd, map[string]any{"providers": entries})
			}
			return printJSON(cmd, map[string]any{"providers": creds})
		}

		out := cmd.OutOrStdout()
		if len(creds) == 0 {
			fmt.Fprintf(out, "No providers configured for @%s. Add them in the web UI under workspace settings.\n", ws)
			return nil
		}

		if !withModels {
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PROVIDER\tNAME\tLAST4\tCREATED")
			for _, c := range creds {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Provider, c.Name, c.LastFour, c.CreatedAt.Format(time.RFC3339))
			}
			_ = tw.Flush()
			return nil
		}

		for _, e := range entries {
			fmt.Fprintf(out, "%s  %s  …%s\n", e.Provider, e.Name, e.LastFour)
			if e.ModelsError != "" {
				fmt.Fprintf(out, "  (could not list models: %s)\n", e.ModelsError)
				continue
			}
			if len(e.Models) == 0 {
				fmt.Fprintln(out, "  (no models reported)")
				continue
			}
			for _, m := range e.Models {
				if m.ContextWindow > 0 {
					fmt.Fprintf(out, "  %s (%d ctx)\n", m.ID, m.ContextWindow)
				} else {
					fmt.Fprintf(out, "  %s\n", m.ID)
				}
			}
		}
		return nil
	},
}

func init() {
	workspaceProvidersCmd.Flags().Bool("models", false, "Also list available models per provider")
}

// parseWorkspaceArg parses a bare `@workspace` reference (no prompt name). The
// promptref grammar requires a /name, so workspace-only refs are parsed here.
func parseWorkspaceArg(arg string) (string, error) {
	if !strings.HasPrefix(arg, "@") {
		return "", fmt.Errorf("workspace must be written as @workspace")
	}
	ws := strings.TrimPrefix(arg, "@")
	if ws == "" || strings.ContainsAny(ws, "/@") {
		return "", fmt.Errorf("invalid workspace %q (expected @workspace)", arg)
	}
	return ws, nil
}
