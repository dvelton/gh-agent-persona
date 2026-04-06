package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var commitCmd = &cobra.Command{
	Use:   "commit <name>",
	Short: "Make a git commit as an agent persona",
	Long: `Makes a single git commit using the specified persona's identity,
without permanently switching your git config. The commit includes
a Co-authored-by trailer for the parent user.`,
	Args: cobra.ExactArgs(1),
	RunE: runCommit,
}

var commitMessage string

func init() {
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message (required)")
	commitCmd.MarkFlagRequired("message")
	rootCmd.AddCommand(commitCmd)
}

func runCommit(cmd *cobra.Command, args []string) error {
	name := args[0]

	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	botName := storage.BotUsername(p.AppSlug)

	// Get the parent user's info for Co-authored-by
	parentUser, _ := getGitHubUsername()
	parentEmail := ""
	if parentUser != "" {
		parentEmail = parentUser + "@users.noreply.github.com"
	}

	// Build commit message with co-authored-by trailer
	msg := commitMessage
	if parentUser != "" {
		msg += fmt.Sprintf("\n\nCo-authored-by: %s <%s>", parentUser, parentEmail)
	}

	// Set environment variables for this commit only
	gitCmd := exec.Command("git", "commit", "-m", msg)
	gitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+botName,
		"GIT_AUTHOR_EMAIL="+p.CommitEmail,
		"GIT_COMMITTER_NAME="+botName,
		"GIT_COMMITTER_EMAIL="+p.CommitEmail,
	)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr

	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	fmt.Printf("\nCommitted as %s\n", botName)
	return nil
}
