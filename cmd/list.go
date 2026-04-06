package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/dvelton/gh-agent-persona/internal/storage"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agent personas",
	RunE:  runList,
}

var showCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show details for an agent persona",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(showCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	personas, err := storage.ListPersonas()
	if err != nil && len(personas) == 0 {
		return err
	}
	if len(personas) == 0 {
		fmt.Println("No agent personas configured.")
		fmt.Println("Create one with: gh agent-persona create <name>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tAPP SLUG\tREPOS\tROLE\tCREATED")
	for _, p := range personas {
		repos := "(none)"
		if len(p.Installations) > 0 {
			var names []string
			for _, inst := range p.Installations {
				names = append(names, inst.Repo)
			}
			repos = strings.Join(names, ", ")
		}
		role := p.Role
		if role == "" {
			role = "(none)"
		}
		created := p.CreatedAt
		if len(created) > 10 {
			created = created[:10]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.AppSlug, repos, role, created)
	}
	w.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	return nil
}

func runShow(cmd *cobra.Command, args []string) error {
	p, err := storage.LoadPersona(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Name:           %s\n", p.Name)
	if p.Role != "" {
		fmt.Printf("Role:           %s\n", p.Role)
	}
	fmt.Printf("App:            %s (ID: %d)\n", p.AppSlug, p.AppID)
	fmt.Printf("Bot user:       %s (ID: %d)\n", storage.BotUsername(p.AppSlug), p.BotUserID)
	fmt.Printf("Commit email:   %s\n", p.CommitEmail)

	if len(p.Installations) > 0 {
		var repos []string
		for _, inst := range p.Installations {
			repos = append(repos, inst.Repo)
		}
		fmt.Printf("Installed on:   %s\n", strings.Join(repos, ", "))
	} else {
		fmt.Printf("Installed on:   (not installed on any repos yet)\n")
	}

	fmt.Printf("Permissions:    %s\n", formatPermissions(p.Permissions))
	fmt.Printf("Created:        %s\n", p.CreatedAt)
	fmt.Printf("Key stored:     %s\n", p.KeyPath)

	return nil
}
