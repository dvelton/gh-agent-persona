package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/dvelton/gh-agent-persona/internal/auth"
	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <persona> -- <command> [args...]",
	Short: "Run a command with an agent persona's identity",
	Long: `Launches a command with the specified persona's git identity and
GitHub API token injected as environment variables. The command
inherits the persona's commit attribution and API access for its
entire lifetime. When the command exits, the environment is clean.

This works with any tool that makes git commits or uses GITHUB_TOKEN
for API calls -- coding agents, automation scripts, CI tools, etc.

Example:
  gh agent-persona run alice -- my-coding-agent
  gh agent-persona run bob -- git commit -m "automated fix"`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    false,
	RunE:                  runRun,
}

var runRepo string

func init() {
	runCmd.Flags().StringVar(&runRepo, "repo", "", "Scope the API token to a specific repo (owner/repo)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	personaName := args[0]

	// Find the "--" separator to split persona args from the command
	cmdArgs := args[1:]
	if len(cmdArgs) == 0 {
		return fmt.Errorf("provide a command to run after \"--\"\n\nUsage: gh agent-persona run <persona> -- <command> [args...]")
	}

	p, err := storage.LoadPersona(personaName)
	if err != nil {
		return err
	}

	botName := storage.BotUsername(p.AppSlug)

	// Generate an installation token
	pemData, err := storage.ReadKey(personaName)
	if err != nil {
		return fmt.Errorf("reading private key: %w", err)
	}
	defer zeroBytes(pemData)

	jwtToken, err := auth.GenerateJWT(p.AppID, pemData)
	if err != nil {
		return fmt.Errorf("generating JWT: %w", err)
	}

	installationID, err := findInstallation(p, jwtToken, runRepo)
	if err != nil {
		return fmt.Errorf("finding installation: %w", err)
	}

	token, expiresAt, err := getInstallationToken(jwtToken, installationID, runRepo)
	if err != nil {
		return fmt.Errorf("generating installation token: %w", err)
	}

	// Build the child process
	child := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	// Inject persona identity via environment variables
	child.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+botName,
		"GIT_AUTHOR_EMAIL="+p.CommitEmail,
		"GIT_COMMITTER_NAME="+botName,
		"GIT_COMMITTER_EMAIL="+p.CommitEmail,
		"GITHUB_TOKEN="+token,
		"GH_TOKEN="+token,
	)

	fmt.Printf("Running as %s\n", botName)
	fmt.Printf("  Token expires: %s\n", expiresAt)
	fmt.Println()

	// Forward signals to the child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			if child.Process != nil {
				child.Process.Signal(sig)
			}
		}
	}()

	err = child.Run()
	signal.Stop(sigCh)
	close(sigCh)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}
