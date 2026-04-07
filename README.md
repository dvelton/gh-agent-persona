# gh-agent-persona

A proof-of-concept `gh` CLI extension that lets you create named AI agent personas on GitHub. Each persona gets its own commit identity, scoped permissions, and audit trail (backed by a GitHub App under the hood).

**This is a personal experiment, not a supported product. Not for use in production environments.** It scratches an itch: giving each of your AI coding agents a distinct, trackable identity on GitHub.

## What It Does

You run multiple AI agents: a coder, a reviewer, a docs writer, maybe a PM bot. Right now they all commit as you, or as a generic "Copilot" identity or similar. This extension gives each one its own name:

```
gh agent-persona create alice --preset reviewer --role "code reviewer"
gh agent-persona create bob --preset coder --role "documentation writer"
```

Each persona shows up in commits and PRs as `your-username-alice-agent[bot]` with verified commits, separate from your personal identity.

## Install

```
gh extension install dvelton/gh-agent-persona
```

Requires the [GitHub CLI](https://cli.github.com/) (`gh`).

## Usage

### Create a persona

```
gh agent-persona create alice --preset reviewer --role "code reviewer"
```

This opens your browser to a one-click GitHub App creation page. Everything is pre-filled. Click "Create GitHub App," and the extension handles the rest (storing credentials, configuring the bot identity, and setting up commit attribution).

After creating the App, install it on your repos by visiting the link shown in the output.

**Available presets:**

Each preset configures both permissions and default instructions. The instructions tell the agent how to behave when injected via `run`.

| Preset | Permissions | Default instructions |
|--------|-------------|---------------------|
| `coder` | Read/write code and PRs, read issues | Implement features, fix bugs, write tests |
| `reviewer` | Read code, write PRs and issues | Review for correctness, security, clarity |
| `docs` | Read/write code and GitHub Pages | Write clear, accurate documentation |
| `ci` | Read code, write checks and actions | Maintain pipelines, fix failing checks |
| `triage` | Write issues and PRs | Label, deduplicate, prioritize incoming work |
| `minimal` | Read code only | (none) |

You can also specify permissions directly:

```
gh agent-persona create alice --permissions contents:read,pull_requests:write,issues:write
```

**Adding instructions at creation time:**

```
gh agent-persona create alice --preset reviewer --instructions "Focus on security issues above all else."
gh agent-persona create alice --preset coder --instructions-file ./my-coder-prompt.md
```

When a preset is used without explicit `--instructions`, the preset's default instructions are applied. You can override or update instructions at any time with the `instructions` command.

### Switch git identity to a persona

```
gh agent-persona use alice
```

Sets `git config user.name` and `user.email` to alice's bot identity in the current repo. All subsequent commits will be attributed to alice.

```
gh agent-persona use --self
```

Reverts to your own git identity.

Add `--global` to apply to all repos.

### Run a command as a persona

```
gh agent-persona run alice -- my-coding-agent
gh agent-persona run bob -- python scripts/auto-review.py
```

Launches the command with the persona's git identity and a fresh `GITHUB_TOKEN` injected as environment variables. Any git commits the command makes will be attributed to the persona. Any GitHub API calls using `GITHUB_TOKEN` or `GH_TOKEN` will authenticate as the persona.

If the persona has instructions, they are injected as:

- `AGENT_PERSONA_INSTRUCTIONS` -- the full instructions text
- `AGENT_PERSONA_INSTRUCTIONS_FILE` -- path to a temp file containing the instructions
- `AGENT_PERSONA_NAME` -- the persona's short name
- `AGENT_PERSONA_ROLE` -- the persona's role description (if set)

The wrapped tool can read these environment variables or ignore them. The temp file is cleaned up when the command exits.

When the command exits, the injected environment is gone -- your own identity is unaffected.

The token expires after 1 hour. For longer sessions, generate a fresh token with `gh agent-persona token`.

### Make a one-off commit as a persona

```
gh agent-persona commit alice -m "Refactored auth middleware"
```

Commits as alice without changing your git config. Adds a `Co-authored-by` trailer for your own account.

### Generate an API token

```
gh agent-persona token alice
```

Generates a short-lived GitHub installation token (1 hour) scoped to alice's permissions. Use this in scripts that call the GitHub API on alice's behalf.

```
eval $(gh agent-persona token alice --export)
# Sets GITHUB_TOKEN=ghs_xxx...
```

### List and inspect

```
gh agent-persona list
gh agent-persona show alice
```

### View or update instructions

```
gh agent-persona instructions alice
```

Prints the persona's current instructions. To update them:

```
gh agent-persona instructions alice --set "You are a security-focused reviewer..."
gh agent-persona instructions alice --from-file ./new-prompt.md
gh agent-persona instructions alice --edit     # opens $EDITOR
gh agent-persona instructions alice --clear    # removes instructions
```

### Delete a persona

```
gh agent-persona delete alice
```

Removes the GitHub App, deletes local credentials, and cleans up git config. Existing commits keep their attribution.

## How It Works

Each persona is a GitHub App created through the [manifest flow](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest). The extension:

1. Generates an App manifest with your chosen name, permissions, and defaults
2. Opens your browser to GitHub's App creation page (one click to confirm)
3. Catches the redirect, exchanges the code for App credentials (ID, private key, etc.)
4. Stores credentials locally at `~/.config/gh-agent-persona/`
5. Looks up the bot user ID so commits are properly attributed to `your-name-alice-agent[bot]`

When you use a persona for commits, the extension sets the git author/committer to the bot's identity and noreply email. GitHub displays these commits with a `[bot]` badge and links to the App's profile.

## Limitations

This is a CLI-only proof of concept. It covers identity and commit attribution but doesn't address:

- **@mention routing** -- you can type `@your-name-alice-agent` in a comment, but there's no autocomplete or notification routing
- **Contribution dashboards** -- no aggregated view of what your agents have done
- **Enterprise governance** -- no org-level policies for persona creation or permissions
- **Provider portability** -- the persona doesn't track which AI model backs it

These gaps would require platform-level changes to address.

## Local Data

Credentials and configuration are stored at `~/.config/gh-agent-persona/`:

```
~/.config/gh-agent-persona/
  personas/
    alice.json       # Persona metadata (app ID, slug, permissions, instructions, etc.)
    bob.json
  keys/
    alice.pem        # Private key for alice's GitHub App
    bob.pem
  instructions/
    alice.md         # Instructions file (editable directly or via the CLI)
    bob.md
```

Private keys are stored with `0600` permissions. Keep this directory secure.

## Naming

GitHub App names must be globally unique. The extension uses the pattern `<your-username>-<persona-name>-agent` to minimize collisions. The GitHub UI will show the full slug (e.g., `dvelton-alice-agent[bot]`), but the extension uses the short name (`alice`) in its own output.
