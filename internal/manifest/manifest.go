package manifest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type AppManifest struct {
	Name               string            `json:"name"`
	URL                string            `json:"url"`
	HookAttributes     HookAttributes    `json:"hook_attributes"`
	RedirectURL        string            `json:"redirect_url"`
	Public             bool              `json:"public"`
	DefaultPermissions map[string]string `json:"default_permissions"`
	DefaultEvents      []string          `json:"default_events"`
	Description        string            `json:"description,omitempty"`
}

type HookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

type ManifestResult struct {
	Code string
}

// RunManifestFlow starts a local server, opens the browser for the manifest
// creation flow, and waits for the redirect callback with the one-time code.
func RunManifestFlow(manifest *AppManifest) (*ManifestResult, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting local server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	state, err := randomState()
	if err != nil {
		return nil, fmt.Errorf("creating manifest state: %w", err)
	}

	manifest.RedirectURL = callbackURL
	manifest.HookAttributes.URL = "https://example.com/no-op"
	manifest.HookAttributes.Active = false

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}

	resultCh := make(chan *ManifestResult, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()

	// Serve a page that auto-submits the manifest form to GitHub
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		page := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Creating Agent Persona...</title></head>
<body>
<p>Redirecting to GitHub to create your agent persona...</p>
<form id="manifest-form" method="post" action="https://github.com/settings/apps/new?state=%s">
  <input type="hidden" name="manifest" value='%s'>
</form>
<script>document.getElementById('manifest-form').submit();</script>
</body>
</html>`, html.EscapeString(state), escapeHTML(string(manifestJSON)))
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})

	// Catch the redirect from GitHub with the one-time code
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.URL.Query().Get("state") != state {
			sendError(errCh, fmt.Errorf("invalid state in callback"))
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			sendError(errCh, fmt.Errorf("no code in callback"))
			http.Error(w, "Missing code parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Done</title></head>
<body>
<p>GitHub App created. You can close this tab and return to your terminal.</p>
</body>
</html>`)
		sendResult(resultCh, &ManifestResult{Code: code})
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			sendError(errCh, err)
		}
	}()

	startURL := fmt.Sprintf("http://127.0.0.1:%d/start", port)
	fmt.Printf("Opening browser to create GitHub App...\n")
	fmt.Printf("If the browser doesn't open, visit: %s\n\n", startURL)
	if err := openBrowser(startURL); err != nil {
		fmt.Printf("Could not open a browser automatically: %v\n\n", err)
	}

	// Wait for result or timeout
	select {
	case result := <-resultCh:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return result, nil
	case err := <-errCh:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return nil, err
	case <-time.After(5 * time.Minute):
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return nil, fmt.Errorf("timed out waiting for GitHub App creation (5 min)")
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("automatic browser launch is not supported on %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func escapeHTML(s string) string {
	// Escape single quotes for the HTML attribute value
	var result strings.Builder
	for _, c := range s {
		switch c {
		case '\'':
			result.WriteString("&#39;")
		case '&':
			result.WriteString("&amp;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '"':
			result.WriteString("&quot;")
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

func sendError(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}

func sendResult(ch chan<- *ManifestResult, result *ManifestResult) {
	select {
	case ch <- result:
	default:
	}
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
