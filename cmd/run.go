package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/dvelton/gh-agent-persona/internal/agentsmd"
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

If the persona has instructions, they are written into AGENTS.md
for the duration of the command and removed on exit. This makes the
instructions available to any coding agent that reads AGENTS.md
(Copilot, Claude Code, Cursor, Codex, Gemini CLI, Windsurf, etc).

Example:
  gh agent-persona run alice -- my-coding-agent
  gh agent-persona run bob -- git commit -m "automated fix"`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    false,
	RunE:                  runRun,
}

var (
	runRepo       string
	runNoAgentsMD bool
)

func init() {
	runCmd.Flags().StringVar(&runRepo, "repo", "", "Scope the API token to a specific repo (owner/repo)")
	runCmd.Flags().BoolVar(&runNoAgentsMD, "no-agents-md", false, "Skip writing persona instructions to AGENTS.md")
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

	// Resolve persona instructions.
	// The .md file is authoritative; fall back to JSON only if no file exists.
	instructions := resolvePersonaInstructions(p)

	var instrTempFile string
	var agentsApplied bool
	var agentsPriorContent []byte
	var agentsPriorExisted bool
	agentsPath := filepath.Join(".", "AGENTS.md")

	if instructions != "" {
		child.Env = append(child.Env, "AGENT_PERSONA_INSTRUCTIONS="+instructions)
		child.Env = append(child.Env, "AGENT_PERSONA_NAME="+p.Name)
		if p.Role != "" {
			child.Env = append(child.Env, "AGENT_PERSONA_ROLE="+p.Role)
		}

		// Write to a unique temp file for tools that prefer file-based config
		if f, err := os.CreateTemp("", fmt.Sprintf("agent-persona-%s-*.md", personaName)); err == nil {
			instrTempFile = f.Name()
			f.Write([]byte(instructions))
			f.Close()
			os.Chmod(instrTempFile, 0600)
			child.Env = append(child.Env, "AGENT_PERSONA_INSTRUCTIONS_FILE="+instrTempFile)
		}

		// Auto-apply instructions to AGENTS.md so coding agents pick them up.
		// Save the prior state so we can restore it on exit (preserving any
		// persistent `apply` or user content).
		if !runNoAgentsMD {
			if data, err := os.ReadFile(agentsPath); err == nil {
				agentsPriorContent = data
				agentsPriorExisted = true
			}
			if err := agentsmd.ApplyToFile(agentsPath, personaName, instructions); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not write AGENTS.md: %v\n", err)
			} else {
				agentsApplied = true
			}
		}
	}

	fmt.Printf("Running as %s\n", botName)
	fmt.Printf("  Token expires: %s\n", expiresAt)
	if agentsApplied {
		fmt.Printf("  Instructions:  written to AGENTS.md (will be removed on exit)\n")
	}
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

	// Clean up: restore AGENTS.md to its prior state and remove temp file
	if agentsApplied {
		if agentsPriorExisted {
			if err := os.WriteFile(agentsPath, agentsPriorContent, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not restore AGENTS.md: %v\n", err)
			}
		} else {
			os.Remove(agentsPath)
		}
	}
	if instrTempFile != "" {
		os.Remove(instrTempFile)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}
