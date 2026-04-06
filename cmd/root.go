package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gh-agent-persona",
	Short: "Manage named AI agent personas on GitHub",
	Long: `gh-agent-persona lets you create, manage, and use named AI agent
personas on GitHub. Each persona is backed by a GitHub App, giving it
a distinct commit identity, scoped permissions, and an audit trail.

This is a proof-of-concept experiment.`,
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

func exitError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+msg+"\n", args...)
	os.Exit(1)
}
