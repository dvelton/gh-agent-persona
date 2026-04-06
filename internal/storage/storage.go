package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Installation struct {
	ID   int64  `json:"id"`
	Repo string `json:"repo"`
}

type Persona struct {
	Name          string            `json:"name"`
	Role          string            `json:"role,omitempty"`
	AppID         int64             `json:"app_id"`
	AppSlug       string            `json:"app_slug"`
	BotUserID     int64             `json:"bot_user_id"`
	CommitEmail   string            `json:"commit_email"`
	Installations []Installation    `json:"installations"`
	Permissions   map[string]string `json:"permissions"`
	CreatedAt     string            `json:"created_at"`
	KeyPath       string            `json:"key_path"`
}

func ConfigDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "gh-agent-persona"), nil
}

func EnsureDirs() error {
	base, err := ConfigDir()
	if err != nil {
		return err
	}
	for _, sub := range []string{"personas", "keys"} {
		if err := os.MkdirAll(filepath.Join(base, sub), 0700); err != nil {
			return err
		}
	}
	return nil
}

func SavePersona(p *Persona) error {
	if err := ValidatePersonaName(p.Name); err != nil {
		return err
	}

	base, err := ConfigDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(base, "personas", personaFilename(p.Name))
	return os.WriteFile(path, data, 0600)
}

func LoadPersona(name string) (*Persona, error) {
	if err := ValidatePersonaName(name); err != nil {
		return nil, err
	}

	base, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, "personas", personaFilename(name))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("persona %q not found", name)
		}
		return nil, err
	}
	var p Persona
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func ListPersonas() ([]*Persona, error) {
	base, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "personas")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var personas []*Persona
	var loadErrors []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		p, err := LoadPersona(name)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		personas = append(personas, p)
	}
	sort.Slice(personas, func(i, j int) bool {
		return personas[i].Name < personas[j].Name
	})
	if len(loadErrors) > 0 {
		return personas, fmt.Errorf("could not load %d persona file(s): %s", len(loadErrors), strings.Join(loadErrors, "; "))
	}
	return personas, nil
}

func DeletePersona(name string) error {
	if err := ValidatePersonaName(name); err != nil {
		return err
	}

	base, err := ConfigDir()
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(base, "personas", personaFilename(name))
	keyPath := filepath.Join(base, "keys", keyFilename(name))

	var errs []string
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("removing private key: %v", err))
	}
	if err := os.Remove(jsonPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("removing persona file: %v", err))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func SaveKey(name string, pemData []byte) (string, error) {
	if err := ValidatePersonaName(name); err != nil {
		return "", err
	}

	base, err := ConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(base, "keys", keyFilename(name))
	if err := os.WriteFile(path, pemData, 0600); err != nil {
		return "", err
	}
	return path, nil
}

func ReadKey(name string) ([]byte, error) {
	if err := ValidatePersonaName(name); err != nil {
		return nil, err
	}

	base, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, "keys", keyFilename(name))
	return os.ReadFile(path)
}

func BotUsername(slug string) string {
	return slug + "[bot]"
}

func BotEmail(botUserID int64, slug string) string {
	return fmt.Sprintf("%d+%s[bot]@users.noreply.github.com", botUserID, slug)
}

func ValidatePersonaName(name string) error {
	if name == "" {
		return fmt.Errorf("persona name cannot be empty")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("persona name cannot start or end with whitespace")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("persona name %q is not allowed", name)
	}
	if strings.ContainsRune(name, filepath.Separator) || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("persona name %q cannot contain path separators", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("persona name %q cannot contain control characters", name)
		}
	}
	return nil
}

func personaFilename(name string) string {
	return name + ".json"
}

func keyFilename(name string) string {
	return name + ".pem"
}
