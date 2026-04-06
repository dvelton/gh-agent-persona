package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2"
	"github.com/dvelton/gh-agent-persona/internal/auth"
	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token <name>",
	Short: "Generate a GitHub API token for an agent persona",
	Long: `Generates a fresh GitHub App installation token for the specified
persona. The token is scoped to the persona's permissions and expires
after 1 hour.

Use --export to output in a format suitable for eval.`,
	Args: cobra.ExactArgs(1),
	RunE: runToken,
}

var (
	tokenRepo   string
	tokenExport bool
)

func init() {
	tokenCmd.Flags().StringVar(&tokenRepo, "repo", "", "Scope token to a specific repo (owner/repo)")
	tokenCmd.Flags().BoolVar(&tokenExport, "export", false, "Output as shell export statement")
	rootCmd.AddCommand(tokenCmd)
}

func runToken(cmd *cobra.Command, args []string) error {
	name := args[0]

	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	// Read the private key
	pemData, err := storage.ReadKey(name)
	if err != nil {
		return fmt.Errorf("reading private key: %w", err)
	}
	defer zeroBytes(pemData)

	// Generate JWT
	jwtToken, err := auth.GenerateJWT(p.AppID, pemData)
	if err != nil {
		return fmt.Errorf("generating JWT: %w", err)
	}

	// Find the installation ID
	installationID, err := findInstallation(p, jwtToken, tokenRepo)
	if err != nil {
		return fmt.Errorf("finding installation: %w", err)
	}

	// Exchange for installation token
	token, expiresAt, err := getInstallationToken(jwtToken, installationID, tokenRepo)
	if err != nil {
		return err
	}

	if tokenExport {
		fmt.Printf("export GITHUB_TOKEN=%s\n", token)
	} else {
		fmt.Println(token)
		fmt.Println()
		fmt.Printf("# Expires: %s\n", expiresAt)
		if tokenRepo != "" {
			fmt.Printf("# Scoped to: %s\n", tokenRepo)
		}
		fmt.Printf("# Permissions: %s\n", formatPermissions(p.Permissions))
	}

	return nil
}

func getInstallationToken(jwtToken string, installationID int64, repo string) (string, string, error) {
	endpoint := fmt.Sprintf("app/installations/%d/access_tokens", installationID)

	apiArgs := []string{"api", "--method", "POST", endpoint,
		"-H", "Authorization: Bearer " + jwtToken,
		"-H", "Accept: application/vnd.github+json",
	}

	if repo != "" {
		_, repoName, err := splitRepo(repo)
		if err != nil {
			return "", "", err
		}
		apiArgs = append(apiArgs, "-f", fmt.Sprintf("repositories[]=%s", repoName))
	}

	stdout, _, err := gh.Exec(apiArgs...)
	if err != nil {
		return "", "", fmt.Errorf("creating installation token: %w", err)
	}

	var resp struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", "", fmt.Errorf("parsing token response: %w", err)
	}
	if resp.Token == "" {
		return "", "", fmt.Errorf("GitHub returned an empty installation token")
	}

	return resp.Token, resp.ExpiresAt, nil
}

func findInstallation(p *storage.Persona, jwtToken, repo string) (int64, error) {
	if repo != "" {
		owner, _, err := splitRepo(repo)
		if err != nil {
			return 0, err
		}
		for _, inst := range p.Installations {
			if strings.EqualFold(inst.Repo, repo) {
				return inst.ID, nil
			}
			if instOwner, _, err := splitRepo(inst.Repo); err == nil && strings.EqualFold(instOwner, owner) {
				return inst.ID, nil
			}
		}
	}

	installations, err := listAppInstallations(jwtToken)
	if err != nil {
		return 0, fmt.Errorf("listing installations: %w\nHave you installed the app? Visit: https://github.com/apps/%s/installations/new", err, p.AppSlug)
	}
	if len(installations) == 0 {
		return 0, fmt.Errorf("no installations found. Install the app at: https://github.com/apps/%s/installations/new", p.AppSlug)
	}

	if repo == "" {
		if len(installations) > 1 {
			return 0, fmt.Errorf("app is installed on multiple accounts; re-run with --repo owner/repo to choose one installation")
		}
		return installations[0].ID, nil
	}

	owner, _, err := splitRepo(repo)
	if err != nil {
		return 0, err
	}
	for _, inst := range installations {
		if strings.EqualFold(inst.Account.Login, owner) {
			return inst.ID, nil
		}
	}

	return 0, fmt.Errorf("no installation found for repo owner %q. Install the app at: https://github.com/apps/%s/installations/new", owner, p.AppSlug)
}

type appInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
}

func listAppInstallations(jwtToken string) ([]appInstallation, error) {
	stdout, _, err := gh.Exec("api", "app/installations",
		"-H", "Authorization: Bearer "+jwtToken,
		"-H", "Accept: application/vnd.github+json",
	)
	if err != nil {
		return nil, err
	}

	var installations []appInstallation
	if err := json.Unmarshal(stdout.Bytes(), &installations); err != nil {
		return nil, fmt.Errorf("parsing installations response: %w", err)
	}
	return installations, nil
}
