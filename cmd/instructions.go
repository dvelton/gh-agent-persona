package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var instructionsCmd = &cobra.Command{
	Use:   "instructions <name>",
	Short: "View or update a persona's instructions",
	Long: `View or update the instructions that define a persona's behavior.
Instructions are injected as environment variables when running commands
with the persona (via AGENT_PERSONA_INSTRUCTIONS and
AGENT_PERSONA_INSTRUCTIONS_FILE).

View instructions:
  gh agent-persona instructions alice

Set instructions inline:
  gh agent-persona instructions alice --set "You are a code reviewer..."

Set instructions from a file:
  gh agent-persona instructions alice --from-file ./reviewer-prompt.md

Open instructions in your editor:
  gh agent-persona instructions alice --edit

Clear instructions:
  gh agent-persona instructions alice --clear`,
	Args: cobra.ExactArgs(1),
	RunE: runInstructions,
}

var (
	instrSet      string
	instrFromFile string
	instrEdit     bool
	instrClear    bool
)

func init() {
	instructionsCmd.Flags().StringVar(&instrSet, "set", "", "Set instructions to this text")
	instructionsCmd.Flags().StringVar(&instrFromFile, "from-file", "", "Set instructions from a file")
	instructionsCmd.Flags().BoolVar(&instrEdit, "edit", false, "Open instructions in $EDITOR")
	instructionsCmd.Flags().BoolVar(&instrClear, "clear", false, "Remove all instructions")
	rootCmd.AddCommand(instructionsCmd)
}

func runInstructions(cmd *cobra.Command, args []string) error {
	name := args[0]

	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	// Count how many mutating flags are set
	mutating := 0
	if instrSet != "" {
		mutating++
	}
	if instrFromFile != "" {
		mutating++
	}
	if instrEdit {
		mutating++
	}
	if instrClear {
		mutating++
	}
	if mutating > 1 {
		return fmt.Errorf("specify only one of --set, --from-file, --edit, or --clear")
	}

	// Clear
	if instrClear {
		p.Instructions = ""
		if err := storage.SavePersona(p); err != nil {
			return fmt.Errorf("saving persona: %w", err)
		}
		if err := storage.SaveInstructions(name, ""); err != nil {
			return fmt.Errorf("saving instructions file: %w", err)
		}
		fmt.Printf("Cleared instructions for %q\n", name)
		return nil
	}

	// Set inline
	if instrSet != "" {
		p.Instructions = instrSet
		if err := storage.SavePersona(p); err != nil {
			return fmt.Errorf("saving persona: %w", err)
		}
		if err := storage.SaveInstructions(name, instrSet); err != nil {
			return fmt.Errorf("saving instructions file: %w", err)
		}
		fmt.Printf("Updated instructions for %q (%d lines)\n", name, strings.Count(instrSet, "\n")+1)
		return nil
	}

	// Set from file
	if instrFromFile != "" {
		data, err := os.ReadFile(instrFromFile)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return fmt.Errorf("file %q is empty", instrFromFile)
		}
		p.Instructions = content
		if err := storage.SavePersona(p); err != nil {
			return fmt.Errorf("saving persona: %w", err)
		}
		if err := storage.SaveInstructions(name, content); err != nil {
			return fmt.Errorf("saving instructions file: %w", err)
		}
		fmt.Printf("Updated instructions for %q from %s (%d lines)\n", name, instrFromFile, strings.Count(content, "\n")+1)
		return nil
	}

	// Edit in $EDITOR
	if instrEdit {
		return editInstructions(p)
	}

	// Default: view
	instructions, fileExists, _ := storage.ReadInstructions(name)
	if !fileExists {
		instructions = p.Instructions
	}
	if instructions == "" {
		fmt.Printf("No instructions set for %q\n", name)
		fmt.Printf("\nSet them with:\n")
		fmt.Printf("  gh agent-persona instructions %s --set \"...\"\n", name)
		fmt.Printf("  gh agent-persona instructions %s --from-file ./prompt.md\n", name)
		fmt.Printf("  gh agent-persona instructions %s --edit\n", name)
		return nil
	}
	fmt.Println(instructions)
	return nil
}

func editInstructions(p *storage.Persona) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return fmt.Errorf("$EDITOR is not set; use --set or --from-file instead")
	}

	instrPath, err := storage.InstructionsPath(p.Name)
	if err != nil {
		return err
	}

	// Ensure the instructions file exists with current content
	existing, fileExists, _ := storage.ReadInstructions(p.Name)
	if !fileExists {
		existing = p.Instructions
	}
	if err := storage.SaveInstructions(p.Name, existing); err != nil {
		return fmt.Errorf("preparing instructions file: %w", err)
	}

	// Parse $EDITOR to handle values like "code --wait" or "emacsclient -c"
	parts := strings.Fields(editor)
	editorArgs := append(parts[1:], instrPath)
	editorCmd := exec.Command(parts[0], editorArgs...)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Read back the edited content
	content, _, err := storage.ReadInstructions(p.Name)
	if err != nil {
		return fmt.Errorf("reading edited instructions: %w", err)
	}

	p.Instructions = content
	if err := storage.SavePersona(p); err != nil {
		return fmt.Errorf("saving persona: %w", err)
	}

	if content == "" {
		fmt.Printf("Instructions cleared for %q\n", p.Name)
	} else {
		fmt.Printf("Updated instructions for %q (%d lines)\n", p.Name, strings.Count(content, "\n")+1)
	}
	return nil
}
