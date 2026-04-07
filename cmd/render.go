package cmd

import (
	"fmt"

	"github.com/dvelton/gh-agent-persona/internal/agentsmd"
	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var renderCmd = &cobra.Command{
	Use:   "render <name>",
	Short: "Output persona instructions in a specific format",
	Long: `Outputs the persona's instructions in a format ready to paste or
pipe into a file. Does not modify any files.

Output as AGENTS.md block (default):
  gh agent-persona render alice

Output raw instructions only:
  gh agent-persona render alice --raw

Pipe into a file you control:
  gh agent-persona render alice >> AGENTS.md
  gh agent-persona render alice --raw > CLAUDE.md`,
	Args: cobra.ExactArgs(1),
	RunE: runRender,
}

var renderRaw bool

func init() {
	renderCmd.Flags().BoolVar(&renderRaw, "raw", false, "Output raw instructions without AGENTS.md block markers")
	rootCmd.AddCommand(renderCmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	name := args[0]

	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	instructions := resolvePersonaInstructions(p)
	if instructions == "" {
		return fmt.Errorf("persona %q has no instructions. Set them with:\n  gh agent-persona instructions %s --set \"...\"", name, name)
	}

	if renderRaw {
		fmt.Println(instructions)
	} else {
		fmt.Println(agentsmd.Render(name, instructions))
	}

	return nil
}
