package main

import (
	"os"

	"github.com/dvelton/gh-agent-persona/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
