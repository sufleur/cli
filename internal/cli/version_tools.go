package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/generator"
	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

// versionToolsCmd is the parent of `sufleur version tools <action>`.
//
// Pins live on a version, not a prompt: they are frozen when the version is
// published, exactly like its files and output schema. That is why this sits
// under `version` rather than `prompt`.
var versionToolsCmd = &cobra.Command{
	Use:           "tools",
	Short:         "Manage the tool contracts a prompt version pins",
	Long:          "Subcommands read and edit the tool contracts pinned by a prompt version, identified as @workspace/name@version.",
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

var versionToolsListCmd = &cobra.Command{
	Use:   "list @workspace/name@version",
	Short: "List the tool contracts a prompt version pins",
	Long: `Lists each pinned contract with the wire name the model sees.

Pins you cannot read are omitted by the registry rather than reported, so this
shows what your credentials can see.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parsePromptVersionRef(args[0])
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		pins, err := client.GetPromptVersionTools(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, pins)
		}

		out := cmd.OutOrStdout()
		if len(pins) == 0 {
			fmt.Fprintln(out, "No tools pinned.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "WIRE NAME\tTOOL\tVERSION\tSTATUS")
		for _, p := range pins {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				p.Alias, p.ToolVersion.Tool.Ref(), p.ToolVersion.Version, p.ToolVersion.Status)
		}
		_ = tw.Flush()
		return nil
	},
}

var versionToolsAddCmd = &cobra.Command{
	Use:   "add @workspace/name@draft @workspace/tool@version [--as wire-name]",
	Short: "Pin a tool contract to a draft prompt version",
	Long: `Pins a tool version to a draft prompt version.

The tool reference may carry a semver constraint ("^1.2.0", "*", "1.2.0") or the
literal "draft". The registry resolves it once, at link time, and stores the
concrete version — a pin never moves afterwards.

--as sets the wire name the model sees. It defaults to the tool's own name, and
exists so two tools that share a bare name can be told apart within one prompt.

Tools in another workspace can be pinned as long as you can read them.`,
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		promptRef, err := parsePromptVersionRef(args[0])
		if err != nil {
			return err
		}
		toolRef, err := parseToolRef(args[1], true)
		if err != nil {
			return err
		}
		if err := validateConstraint(toolRef.Version); err != nil {
			return err
		}
		alias, _ := cmd.Flags().GetString("as")
		if alias != "" && !generator.AliasRe.MatchString(alias) {
			return fmt.Errorf(
				"wire name %q must match %s — the model sees it verbatim",
				alias, generator.AliasRe.String())
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		if err := client.LinkTool(cmd.Context(), promptRef.Workspace, promptRef.Name, promptRef.Version,
			toolRef.Workspace, toolRef.Name, toolRef.Version, alias); err != nil {
			return mapBearer(err)
		}

		return reportPins(cmd, client, promptRef, fmt.Sprintf("Pinned @%s/%s to", toolRef.Workspace, toolRef.Name))
	},
}

var versionToolsRenameCmd = &cobra.Command{
	Use:   "rename @workspace/name@draft @workspace/tool --as wire-name",
	Short: "Change the wire name a pinned tool is exposed under",
	Long: `Renames the wire name the model sees for an already-pinned tool.

The tool reference carries no version here: a prompt version pins at most one
version of any given tool, so the tool alone identifies the pin.`,
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		promptRef, err := parsePromptVersionRef(args[0])
		if err != nil {
			return err
		}
		toolRef, err := parseToolRef(args[1], false)
		if err != nil {
			return err
		}
		alias, _ := cmd.Flags().GetString("as")
		if alias == "" {
			return fmt.Errorf("--as is required")
		}
		if !generator.AliasRe.MatchString(alias) {
			return fmt.Errorf(
				"wire name %q must match %s — the model sees it verbatim",
				alias, generator.AliasRe.String())
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		if err := client.UpdateToolLink(cmd.Context(), promptRef.Workspace, promptRef.Name, promptRef.Version,
			toolRef.Workspace, toolRef.Name, alias); err != nil {
			return mapBearer(err)
		}

		return reportPins(cmd, client, promptRef, fmt.Sprintf("Renamed @%s/%s to %q on", toolRef.Workspace, toolRef.Name, alias))
	},
}

var versionToolsRemoveCmd = &cobra.Command{
	Use:   "remove @workspace/name@draft @workspace/tool",
	Short: "Remove a pinned tool from a draft prompt version",
	Long: `Removes a pin from a draft prompt version.

This is a local, one-command-reversible edit to a draft, and the registry refuses
it outright on a published version — unlike detaching a prompt from a shared
collection, which the CLI deliberately does not expose.`,
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		promptRef, err := parsePromptVersionRef(args[0])
		if err != nil {
			return err
		}
		toolRef, err := parseToolRef(args[1], false)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		if err := client.UnlinkTool(cmd.Context(), promptRef.Workspace, promptRef.Name, promptRef.Version,
			toolRef.Workspace, toolRef.Name); err != nil {
			return mapBearer(err)
		}

		return reportPins(cmd, client, promptRef, fmt.Sprintf("Removed @%s/%s from", toolRef.Workspace, toolRef.Name))
	},
}

// reportPins re-reads the version's pins after a write, so the caller sees the
// resulting state — including the version a constraint resolved to, which is
// the whole point of pinning and is not knowable from the request.
func reportPins(cmd *cobra.Command, client *userapi.Client, ref promptref.PromptRef, headline string) error {
	pins, err := client.GetPromptVersionTools(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
	if err != nil {
		return mapBearer(err)
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		return printJSON(cmd, pins)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s @%s/%s@%s.\n", headline, ref.Workspace, ref.Name, ref.Version)
	if len(pins) == 0 {
		fmt.Fprintln(out, "\nNo tools pinned.")
		return nil
	}
	fmt.Fprintln(out, "\npinned tools:")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, p := range pins {
		fmt.Fprintf(tw, "  %s\t%s@%s\t%s\n", p.Alias, p.ToolVersion.Tool.Ref(), p.ToolVersion.Version, p.ToolVersion.Status)
	}
	_ = tw.Flush()
	return nil
}

// parsePromptVersionRef parses @workspace/name@version for a prompt.
func parsePromptVersionRef(arg string) (promptref.PromptRef, error) {
	ref, err := promptref.ParseRef(arg)
	if err != nil {
		return promptref.PromptRef{}, err
	}
	if ref.IsCollection {
		return promptref.PromptRef{}, fmt.Errorf("%q is a collection reference; expected a prompt", arg)
	}
	if ref.Version == "" {
		return promptref.PromptRef{}, fmt.Errorf("version is required (use @workspace/name@version, or @workspace/name@draft)")
	}
	return ref, nil
}

func init() {
	versionToolsAddCmd.Flags().String("as", "", "Wire name the model sees (defaults to the tool's own name)")
	versionToolsRenameCmd.Flags().String("as", "", "New wire name the model sees")

	versionToolsCmd.AddCommand(
		versionToolsListCmd,
		versionToolsAddCmd,
		versionToolsRenameCmd,
		versionToolsRemoveCmd,
	)
}
