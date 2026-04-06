package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set git identity to an agent persona",
	Long: `Sets the git user.name and user.email for the current repo (or globally
with --global) to the specified agent persona's bot identity.

Use --self to revert to your own identity.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUse,
}

var (
	useGlobal bool
	useSelf   bool
)

func init() {
	useCmd.Flags().BoolVar(&useGlobal, "global", false, "Apply to global git config")
	useCmd.Flags().BoolVar(&useSelf, "self", false, "Revert to your own git identity")
	rootCmd.AddCommand(useCmd)
}

func runUse(cmd *cobra.Command, args []string) error {
	if useSelf {
		return revertToSelf()
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a persona name, or use --self to revert")
	}

	name := args[0]
	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	botName := storage.BotUsername(p.AppSlug)

	scope := "local"
	if useGlobal {
		scope = "global"
	}

	if err := setGitConfig("user.name", botName); err != nil {
		return fmt.Errorf("setting git user.name: %w", err)
	}
	if err := setGitConfig("user.email", p.CommitEmail); err != nil {
		return fmt.Errorf("setting git user.email: %w", err)
	}

	fmt.Printf("Git identity set to %s (%s)\n", botName, scope)
	fmt.Printf("  user.name:  %s\n", botName)
	fmt.Printf("  user.email: %s\n", p.CommitEmail)
	fmt.Println()
	fmt.Printf("To revert: gh agent-persona use --self\n")
	return nil
}

func revertToSelf() error {
	if err := setGitConfig("user.name", ""); err != nil {
		return fmt.Errorf("reverting git user.name: %w", err)
	}
	if err := setGitConfig("user.email", ""); err != nil {
		return fmt.Errorf("reverting git user.email: %w", err)
	}

	fmt.Println("Reverted git identity to default.")
	return nil
}

func setGitConfig(key, value string) error {
	args := []string{"config"}
	if useGlobal {
		args = append(args, "--global")
	}
	if value == "" {
		args = append(args, "--unset", key)
	} else {
		args = append(args, key, value)
	}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if value == "" && (strings.Contains(trimmed, "No such section or key") || strings.Contains(trimmed, "key does not contain a section")) {
			return nil
		}
		return fmt.Errorf("%s: %s", err, trimmed)
	}
	return nil
}

func getGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
