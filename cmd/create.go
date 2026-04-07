package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/dvelton/gh-agent-persona/internal/manifest"
	"github.com/dvelton/gh-agent-persona/internal/presets"
	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new agent persona",
	Long: `Creates a new agent persona backed by a GitHub App.

The command opens your browser to complete a one-click GitHub App registration,
then stores credentials locally for later use.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

var (
	createRole             string
	createRepos            string
	createPermissions      string
	createPreset           string
	createPrivate          bool
	createInstructions     string
	createInstructionsFile string
)

func init() {
	createCmd.Flags().StringVar(&createRole, "role", "", "Description of what this agent does")
	createCmd.Flags().StringVar(&createRepos, "repos", "", "Comma-separated repos to install on (e.g., owner/repo)")
	createCmd.Flags().StringVar(&createPermissions, "permissions", "", "Comma-separated permission:level pairs")
	createCmd.Flags().StringVar(&createPreset, "preset", "", "Permission preset: "+strings.Join(presets.Names(), ", "))
	createCmd.Flags().BoolVar(&createPrivate, "private", true, "Make the GitHub App private")
	createCmd.Flags().StringVar(&createInstructions, "instructions", "", "Instructions that define this persona's behavior")
	createCmd.Flags().StringVar(&createInstructionsFile, "instructions-file", "", "Path to a file containing instructions")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := storage.ValidatePersonaName(name); err != nil {
		return err
	}

	if err := storage.EnsureDirs(); err != nil {
		return fmt.Errorf("setting up config directory: %w", err)
	}

	// Check if persona already exists locally
	if _, err := storage.LoadPersona(name); err == nil {
		return fmt.Errorf("persona %q already exists. Use 'delete' first to recreate it", name)
	}

	// Get current GitHub username
	username, err := getGitHubUsername()
	if err != nil {
		return fmt.Errorf("getting GitHub username: %w", err)
	}

	// Build permissions map
	perms, err := resolvePermissions()
	if err != nil {
		return err
	}
	repos, err := parseRepoList(createRepos)
	if err != nil {
		return err
	}

	// Resolve instructions
	instructions, err := resolveInstructions()
	if err != nil {
		return err
	}

	appName := buildAppName(username, name)
	description := createRole
	if description == "" {
		description = fmt.Sprintf("Agent persona for %s", username)
	}

	m := &manifest.AppManifest{
		Name:               appName,
		URL:                fmt.Sprintf("https://github.com/%s", username),
		Public:             !createPrivate,
		DefaultPermissions: perms,
		DefaultEvents:      []string{},
		Description:        description,
	}

	// Run the manifest flow (opens browser, waits for callback)
	result, err := manifest.RunManifestFlow(m)
	if err != nil {
		return fmt.Errorf("manifest flow: %w", err)
	}

	// Exchange the code for app credentials
	appData, err := exchangeManifestCode(result.Code)
	if err != nil {
		return fmt.Errorf("exchanging manifest code: %w", err)
	}

	// Save the private key
	pemData := []byte(appData.PEM)
	appData.PEM = ""
	defer zeroBytes(pemData)

	keyPath, err := storage.SaveKey(name, pemData)
	if err != nil {
		return fmt.Errorf("saving private key: %w", err)
	}

	// Look up the bot user ID
	botUserID, err := getBotUserID(appData.Slug)
	if err != nil {
		_ = os.Remove(keyPath)
		return fmt.Errorf("looking up bot user ID for %s[bot]: %w (the GitHub App was created, but local setup stopped; delete it manually at https://github.com/settings/apps/%s if needed)", appData.Slug, err, appData.Slug)
	}

	commitEmail := storage.BotEmail(botUserID, appData.Slug)

	persona := &storage.Persona{
		Name:          name,
		Role:          createRole,
		Instructions:  instructions,
		AppID:         appData.ID,
		AppSlug:       appData.Slug,
		BotUserID:     botUserID,
		CommitEmail:   commitEmail,
		Installations: []storage.Installation{},
		Permissions:   perms,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		KeyPath:       keyPath,
	}

	if err := storage.SavePersona(persona); err != nil {
		_ = os.Remove(keyPath)
		return fmt.Errorf("saving persona: %w", err)
	}

	// Save instructions to a separate file for easy editing
	if instructions != "" {
		if err := storage.SaveInstructions(name, instructions); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save instructions file: %v\n", err)
		}
	}

	// If repos were specified, print install instructions
	fmt.Printf("\nCreated agent persona %q\n\n", name)
	fmt.Printf("  GitHub App:    %s\n", appData.Slug)
	fmt.Printf("  Bot identity:  %s\n", storage.BotUsername(appData.Slug))
	fmt.Printf("  Commit email:  %s\n", commitEmail)
	fmt.Printf("  Permissions:   %s\n", formatPermissions(perms))
	if instructions != "" {
		lines := strings.Count(instructions, "\n") + 1
		fmt.Printf("  Instructions:  %d lines (view with: gh agent-persona instructions %s)\n", lines, name)
	}
	fmt.Println()

	if len(repos) > 0 {
		installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", appData.Slug)
		fmt.Printf("  Install on your repos at: %s\n", installURL)
		fmt.Printf("  Requested repos: %s\n", strings.Join(repos, ", "))
	} else {
		installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", appData.Slug)
		fmt.Printf("  Install on repos at: %s\n", installURL)
	}

	fmt.Println()
	fmt.Println("  To use this persona for commits:")
	fmt.Printf("    gh agent-persona use %s\n", name)
	fmt.Println()
	fmt.Println("  To generate an API token:")
	fmt.Printf("    gh agent-persona token %s\n", name)

	return nil
}

func getGitHubUsername() (string, error) {
	stdout, _, err := gh.Exec("api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	username := strings.TrimSpace(stdout.String())
	if username == "" {
		return "", fmt.Errorf("GitHub API returned an empty username")
	}
	return username, nil
}

func resolvePermissions() (map[string]string, error) {
	// Preset takes priority if specified
	if createPreset != "" {
		p, ok := presets.Presets[createPreset]
		if !ok {
			return nil, fmt.Errorf("unknown preset %q. Available: %s", createPreset, strings.Join(presets.Names(), ", "))
		}
		perms := make(map[string]string)
		for k, v := range p {
			perms[k] = v
		}
		// Overlay any explicit permissions
		if createPermissions != "" {
			overlay, err := parsePermissions(createPermissions)
			if err != nil {
				return nil, err
			}
			for k, v := range overlay {
				perms[k] = v
			}
		}
		return perms, nil
	}

	if createPermissions != "" {
		return parsePermissions(createPermissions)
	}

	// Defaults
	return map[string]string{
		"contents":      "read",
		"pull_requests": "write",
		"metadata":      "read",
	}, nil
}

func parsePermissions(s string) (map[string]string, error) {
	perms := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			return nil, fmt.Errorf("permissions cannot contain empty entries")
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid permission format %q, expected name:level", pair)
		}
		name := strings.TrimSpace(parts[0])
		level := strings.TrimSpace(parts[1])
		if name == "" {
			return nil, fmt.Errorf("permission name cannot be empty")
		}
		switch level {
		case "read", "write", "admin":
		default:
			return nil, fmt.Errorf("invalid permission level %q for %s", level, name)
		}
		perms[name] = level
	}
	return perms, nil
}

func formatPermissions(perms map[string]string) string {
	if len(perms) == 0 {
		return "(none)"
	}

	keys := make([]string, 0, len(perms))
	for k := range perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := perms[k]
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, ", ")
}

type appCredentials struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	PEM  string `json:"pem"`
}

func exchangeManifestCode(code string) (*appCredentials, error) {
	stdout, _, err := gh.Exec("api", "--method", "POST",
		fmt.Sprintf("app-manifests/%s/conversions", code),
		"-H", "Accept: application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	var creds appCredentials
	if err := json.Unmarshal(stdout.Bytes(), &creds); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if creds.ID == 0 {
		return nil, fmt.Errorf("unexpected response: missing app ID")
	}
	if creds.Slug == "" {
		return nil, fmt.Errorf("unexpected response: missing app slug")
	}
	if creds.PEM == "" {
		return nil, fmt.Errorf("unexpected response: missing private key")
	}
	return &creds, nil
}

func getBotUserID(slug string) (int64, error) {
	botEndpoint := fmt.Sprintf("users/%s%%5Bbot%%5D", slug)
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		stdout, _, err := gh.Exec("api", botEndpoint, "--jq", ".id")
		if err == nil {
			var id int64
			if parseErr := json.Unmarshal(stdout.Bytes(), &id); parseErr != nil {
				return 0, fmt.Errorf("parsing bot user ID: %w", parseErr)
			}
			if id == 0 {
				return 0, fmt.Errorf("GitHub returned bot user ID 0 for %s[bot]", slug)
			}
			return id, nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return 0, lastErr
}

func parseRepoList(input string) ([]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}

	repos := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(input, ",") {
		repo := strings.TrimSpace(item)
		if repo == "" {
			return nil, fmt.Errorf("repos cannot contain empty entries")
		}
		if _, _, err := splitRepo(repo); err != nil {
			return nil, err
		}
		if _, ok := seen[repo]; ok {
			continue
		}
		seen[repo] = struct{}{}
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos, nil
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(repo), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repo must be in owner/repo format")
	}
	return parts[0], parts[1], nil
}

func buildAppName(username, personaName string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		case r == ' ':
			return '-'
		default:
			return '-'
		}
	}, personaName)
	sanitized = strings.Trim(sanitized, "-_")
	if sanitized == "" {
		sanitized = "persona"
	}
	return fmt.Sprintf("%s-%s-agent", username, sanitized)
}

func resolveInstructions() (string, error) {
	// Explicit --instructions flag takes priority
	if createInstructions != "" && createInstructionsFile != "" {
		return "", fmt.Errorf("cannot specify both --instructions and --instructions-file")
	}
	if createInstructions != "" {
		return createInstructions, nil
	}
	if createInstructionsFile != "" {
		data, err := os.ReadFile(createInstructionsFile)
		if err != nil {
			return "", fmt.Errorf("reading instructions file: %w", err)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return "", fmt.Errorf("instructions file %q is empty", createInstructionsFile)
		}
		return content, nil
	}
	// Fall back to preset instructions if a preset was specified
	if createPreset != "" {
		return presets.GetInstructions(createPreset), nil
	}
	return "", nil
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
