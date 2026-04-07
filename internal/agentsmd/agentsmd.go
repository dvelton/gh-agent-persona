package agentsmd

import (
	"fmt"
	"os"
	"strings"
)

const (
	blockStart = "<!-- gh-agent-persona:start %s -->"
	blockEnd   = "<!-- gh-agent-persona:end -->"
)

func markerStart(name string) string {
	return fmt.Sprintf(blockStart, name)
}

// ApplyToFile writes persona instructions into an AGENTS.md file using
// delimited block markers. If the file already exists, the persona block
// is inserted or replaced without disturbing other content. If a different
// persona's block exists, it is replaced.
func ApplyToFile(path, personaName, instructions string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	block := buildBlock(personaName, instructions)
	var result string

	if len(existing) == 0 {
		result = block + "\n"
	} else {
		content := string(existing)
		cleaned := removeAnyBlock(content)
		if strings.TrimSpace(cleaned) == "" {
			result = block + "\n"
		} else {
			result = strings.TrimRight(cleaned, "\n") + "\n\n" + block + "\n"
		}
	}

	return os.WriteFile(path, []byte(result), 0644)
}

// ClearFromFile removes the persona block from an AGENTS.md file.
// If the file is empty after removal, it is deleted.
func ClearFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cleaned := removeAnyBlock(string(data))

	// Only delete if truly empty after block removal
	if strings.TrimSpace(cleaned) == "" {
		return os.Remove(path)
	}

	return os.WriteFile(path, []byte(cleaned), 0644)
}

// HasBlock checks whether an AGENTS.md file contains a persona block.
func HasBlock(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return extractPersonaName(string(data))
}

// Render returns the formatted AGENTS.md block content for a persona,
// without writing it anywhere.
func Render(personaName, instructions string) string {
	return buildBlock(personaName, instructions)
}

func buildBlock(personaName, instructions string) string {
	return markerStart(personaName) + "\n" + strings.TrimSpace(instructions) + "\n" + blockEnd
}

func removeAnyBlock(content string) string {
	for {
		startIdx := findBlockStart(content)
		if startIdx < 0 {
			return content
		}
		endIdx := strings.Index(content[startIdx:], blockEnd)
		if endIdx < 0 {
			return content
		}
		endIdx += startIdx + len(blockEnd)
		// Consume trailing newlines after the block
		for endIdx < len(content) && content[endIdx] == '\n' {
			endIdx++
		}
		content = content[:startIdx] + content[endIdx:]
	}
}

func findBlockStart(content string) int {
	prefix := "<!-- gh-agent-persona:start "
	return strings.Index(content, prefix)
}

func extractPersonaName(content string) (string, bool) {
	prefix := "<!-- gh-agent-persona:start "
	idx := strings.Index(content, prefix)
	if idx < 0 {
		return "", false
	}
	rest := content[idx+len(prefix):]
	endIdx := strings.Index(rest, " -->")
	if endIdx < 0 {
		return "", false
	}
	return rest[:endIdx], true
}
