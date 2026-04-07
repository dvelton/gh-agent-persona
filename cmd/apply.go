package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvelton/gh-agent-persona/internal/agentsmd"
	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply <name>",
	Short: "Write persona instructions into AGENTS.md",
	Long: `Writes the persona's instructions into an AGENTS.md file in the
current directory (or the directory specified by --dir). AGENTS.md is the
cross-tool standard read by Copilot, Claude Code, Cursor, Codex, Gemini
CLI, Windsurf, and others.

The persona's instructions are wrapped in comment markers so they can
be inserted, swapped, or removed without disturbing other content in
the file.

Apply a persona:
  gh agent-persona apply alice

Switch to a different persona:
  gh agent-persona apply bob

Remove persona instructions:
  gh agent-persona apply --clear

Check what's currently applied:
  gh agent-persona apply --status`,
	Args: cobra.MaximumNArgs(1),
	RunE: runApply,
}

var (
	applyDir    string
	applyClear  bool
	applyStatus bool
)

func init() {
	applyCmd.Flags().StringVar(&applyDir, "dir", ".", "Directory containing AGENTS.md")
	applyCmd.Flags().BoolVar(&applyClear, "clear", false, "Remove persona instructions from AGENTS.md")
	applyCmd.Flags().BoolVar(&applyStatus, "status", false, "Show which persona is currently applied")
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	agentsPath := filepath.Join(applyDir, "AGENTS.md")

	if applyStatus {
		name, found := agentsmd.HasBlock(agentsPath)
		if !found {
			fmt.Println("No persona currently applied to AGENTS.md")
		} else {
			fmt.Printf("Persona %q is applied to %s\n", name, agentsPath)
		}
		return nil
	}

	if applyClear {
		if err := agentsmd.ClearFromFile(agentsPath); err != nil {
			return fmt.Errorf("clearing persona from AGENTS.md: %w", err)
		}
		fmt.Println("Removed persona instructions from AGENTS.md")
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a persona name, or use --clear / --status")
	}

	name := args[0]
	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	instructions := resolvePersonaInstructions(p)
	if instructions == "" {
		return fmt.Errorf("persona %q has no instructions. Set them with:\n  gh agent-persona instructions %s --set \"...\"", name, name)
	}

	// Check if a different persona is already applied
	prevName, hadPrev := agentsmd.HasBlock(agentsPath)

	if err := agentsmd.ApplyToFile(agentsPath, name, instructions); err != nil {
		return fmt.Errorf("writing AGENTS.md: %w", err)
	}

	if hadPrev && prevName != name {
		fmt.Printf("Replaced persona %q with %q in %s\n", prevName, name, agentsPath)
	} else {
		fmt.Printf("Applied persona %q to %s\n", name, agentsPath)
	}
	warnIfUntracked(agentsPath)
	return nil
}

func resolvePersonaInstructions(p *storage.Persona) string {
	instructions, fileExists, _ := storage.ReadInstructions(p.Name)
	if !fileExists {
		instructions = p.Instructions
	}
	return instructions
}

func warnIfUntracked(path string) {
	// Check if we're in a git repo and if AGENTS.md is gitignored
	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}
	dir := filepath.Dir(absPath)

	// Check for .git directory
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		// Walk up to find .git
		for {
			parent := filepath.Dir(dir)
			if parent == dir {
				return // not in a git repo
			}
			dir = parent
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				break
			}
		}
	}

	fmt.Println()
	fmt.Println("  Note: AGENTS.md is in your working tree. To avoid committing it:")
	fmt.Println("    echo AGENTS.md >> .gitignore")
	fmt.Println()
	fmt.Println("  To remove when done:")
	fmt.Println("    gh agent-persona apply --clear")
}
