package cmd

import (
	"fmt"

	"github.com/cli/go-gh/v2"
	"github.com/dvelton/gh-agent-persona/internal/auth"
	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an agent persona and uninstall its GitHub App",
	Long: `Deletes a persona locally, uninstalls the GitHub App from any accounts
it can reach, and cleans up git config.

Existing commits by the persona will retain their attribution, but the
bot profile link will become inactive. GitHub App registrations still need
to be deleted manually in the browser.`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

var deleteForce bool

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	p, err := storage.LoadPersona(name)
	if err != nil {
		return err
	}

	if !deleteForce {
		fmt.Printf("This will:\n")
		fmt.Printf("  - Uninstall the GitHub App %q from any accounts this tool can reach\n", p.AppSlug)
		fmt.Printf("  - Leave the GitHub App registration in place for manual deletion in the browser\n")
		fmt.Printf("  - Remove %s's credentials from local storage\n", name)
		fmt.Printf("  - Existing commits by %s will keep their attribution\n", name)
		fmt.Println()
		fmt.Print("Proceed? [y/N] ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	manualDeleteURL := fmt.Sprintf("https://github.com/settings/apps/%s", p.AppSlug)

	// GitHub does not expose an API to delete the app registration itself.
	// We can only uninstall existing installations, then point the user at the UI.
	fmt.Printf("Uninstalling GitHub App %s from any detected installations...\n", p.AppSlug)
	pemData, readErr := storage.ReadKey(name)
	if readErr != nil {
		fmt.Printf("Warning: could not read private key: %v\n", readErr)
		fmt.Printf("Delete the app manually at: %s\n", manualDeleteURL)
	} else {
		defer zeroBytes(pemData)

		jwtToken, jwtErr := auth.GenerateJWT(p.AppID, pemData)
		if jwtErr != nil {
			fmt.Printf("Warning: could not generate JWT: %v\n", jwtErr)
			fmt.Printf("Delete the app manually at: %s\n", manualDeleteURL)
		} else {
			installations, listErr := listAppInstallations(jwtToken)
			if listErr != nil {
				fmt.Printf("Warning: could not list installations: %v\n", listErr)
			} else if len(installations) == 0 {
				fmt.Println("No installations found.")
			} else {
				for _, inst := range installations {
					_, _, apiErr := gh.Exec("api", "--method", "DELETE", fmt.Sprintf("app/installations/%d", inst.ID),
						"-H", "Authorization: Bearer "+jwtToken,
						"-H", "Accept: application/vnd.github+json",
					)
					if apiErr != nil {
						fmt.Printf("Warning: could not uninstall from %s: %v\n", inst.Account.Login, apiErr)
						continue
					}
					fmt.Printf("Uninstalled from %s.\n", inst.Account.Login)
				}
			}
		}
	}

	// Clean up git config if it matches this persona
	if err := cleanGitConfig(p); err != nil {
		fmt.Printf("Warning: could not clean git config: %v\n", err)
	}

	// Delete local files
	if err := storage.DeletePersona(name); err != nil {
		return fmt.Errorf("cleaning up local files: %w", err)
	}

	fmt.Printf("Delete the GitHub App registration in the browser if you no longer need it: %s\n", manualDeleteURL)
	fmt.Printf("Deleted persona %q.\n", name)
	return nil
}

func cleanGitConfig(p *storage.Persona) error {
	botName := storage.BotUsername(p.AppSlug)
	currentName, _ := getGitConfig("user.name")
	if currentName == botName {
		if err := setGitConfig("user.name", ""); err != nil {
			return err
		}
		if err := setGitConfig("user.email", ""); err != nil {
			return err
		}
		fmt.Println("Reverted git config to default identity.")
	}
	return nil
}
