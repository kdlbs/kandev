package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
)

const defaultGitHubHost = "github.com"

// GHAccount is one login stored by the host gh CLI. It contains no token.
type GHAccount struct {
	Host     string `json:"host"`
	Login    string `json:"login"`
	Active   bool   `json:"active"`
	State    string `json:"state"`
	Selected bool   `json:"selected"`
}

type ghAccountCommandRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type systemGHAccountRunner struct{}

func (systemGHAccountRunner) Run(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	runErr, execCtxErr := subproc.RunGHAfterAcquire(ctx, resolveGHExecTimeout(ctx), func(execCtx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(execCtx, "gh", args...)
		cmd.Env = githubCLIEnvironment(os.Environ())
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		return cmd
	})
	if runErr == nil {
		return stdout.String(), nil
	}
	if execCtxErr != nil && (errors.Is(execCtxErr, context.Canceled) || errors.Is(execCtxErr, context.DeadlineExceeded)) {
		return "", execCtxErr
	}
	// Authentication commands may print tokens or credential locations. Keep
	// diagnostics account-specific without reflecting subprocess output.
	return "", fmt.Errorf("gh %s failed: %w", firstArg(args), runErr)
}

func githubCLIEnvironment(source []string) []string {
	filtered := make([]string, 0, len(source))
	for _, item := range source {
		key, _, ok := strings.Cut(item, "=")
		if ok && (strings.EqualFold(key, "GH_TOKEN") || strings.EqualFold(key, "GITHUB_TOKEN")) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

type ghAuthStatus struct {
	Hosts map[string][]struct {
		Active bool   `json:"active"`
		Host   string `json:"host"`
		Login  string `json:"login"`
		State  string `json:"state"`
	} `json:"hosts"`
}

// ListGHAccounts returns every account known to gh without changing its
// active account. Ambient token variables are stripped so they cannot hide
// the accounts stored in gh's config.
func ListGHAccounts(ctx context.Context) ([]GHAccount, error) {
	return listGHAccounts(ctx, systemGHAccountRunner{})
}

func listGHAccounts(ctx context.Context, runner ghAccountCommandRunner) ([]GHAccount, error) {
	raw, err := runner.Run(ctx, "auth", "status", "--json", "hosts")
	if err != nil {
		return nil, fmt.Errorf("list gh accounts: %w", err)
	}
	var status ghAuthStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return nil, fmt.Errorf("decode gh accounts: %w", err)
	}
	accounts := make([]GHAccount, 0)
	for host, entries := range status.Hosts {
		for _, entry := range entries {
			accountHost := entry.Host
			if accountHost == "" {
				accountHost = host
			}
			if accountHost == "" || entry.Login == "" {
				continue
			}
			accounts = append(accounts, GHAccount{
				Host:   accountHost,
				Login:  entry.Login,
				Active: entry.Active,
				State:  entry.State,
			})
		}
	}
	return accounts, nil
}

// ResolveGHAccountToken returns the token for an exact host/login pair. It
// never changes gh's active account and never persists the returned token.
func ResolveGHAccountToken(ctx context.Context, host, login string) (string, error) {
	return resolveGHAccountToken(ctx, systemGHAccountRunner{}, host, login)
}

func resolveGHAccountToken(ctx context.Context, runner ghAccountCommandRunner, host, login string) (string, error) {
	host = strings.TrimSpace(host)
	login = strings.TrimSpace(login)
	if host == "" {
		host = defaultGitHubHost
	}
	if host != defaultGitHubHost {
		return "", fmt.Errorf("unsupported GitHub host %q", host)
	}
	if login == "" {
		return "", fmt.Errorf("GitHub login is required")
	}
	raw, err := runner.Run(ctx, "auth", "token", "--hostname", host, "--user", login)
	if err != nil {
		return "", fmt.Errorf("resolve gh account %s@%s: %w", login, host, err)
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("resolve gh account %s@%s: empty token", login, host)
	}
	return token, nil
}
