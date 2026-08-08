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
	"sync"

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
		if out := stdout.String(); strings.TrimSpace(out) != "" {
			return out, nil
		}
		return stderr.String(), nil
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

var cachedGHTokenUserSupport = sync.OnceValues(func() (bool, error) {
	help, err := (systemGHAccountRunner{}).Run(
		context.Background(), "auth", "token", "--help",
	)
	if err != nil {
		return false, err
	}
	return strings.Contains(help, "--user"), nil
})

// ListGHAccounts returns every account known to gh without changing its
// active account. Ambient token variables are stripped so they cannot hide
// the accounts stored in gh's config.
func ListGHAccounts(ctx context.Context) ([]GHAccount, error) {
	return listGHAccounts(ctx, systemGHAccountRunner{})
}

func listGHAccounts(ctx context.Context, runner ghAccountCommandRunner) ([]GHAccount, error) {
	raw, err := runner.Run(ctx, "auth", "status", "--json", "hosts")
	if err != nil {
		raw, err = runner.Run(ctx, "auth", "status")
		if err != nil {
			return nil, fmt.Errorf("list gh accounts: %w", err)
		}
		accounts := parseLegacyGHAuthStatus(raw)
		if len(accounts) == 0 {
			return nil, errors.New("list gh accounts: unsupported gh auth status output")
		}
		return accounts, nil
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

func parseLegacyGHAuthStatus(raw string) []GHAccount {
	accounts := make([]GHAccount, 0)
	host := ""
	currentAccount := -1
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == line && trimmed != "" && !strings.ContainsAny(trimmed, " \t") {
			host = trimmed
			currentAccount = -1
			continue
		}
		if login := legacyStatusLogin(trimmed, host); login != "" {
			accounts = append(accounts, GHAccount{
				Host: host, Login: login, Active: true, State: "success",
			})
			currentAccount = len(accounts) - 1
			continue
		}
		if currentAccount >= 0 && strings.HasPrefix(trimmed, "- Active account:") {
			accounts[currentAccount].Active = strings.EqualFold(
				strings.TrimSpace(strings.TrimPrefix(trimmed, "- Active account:")),
				"true",
			)
		}
	}
	return accounts
}

func legacyStatusLogin(line, host string) string {
	if host == "" {
		return ""
	}
	for _, marker := range []string{
		"Logged in to " + host + " account ",
		"Logged in to " + host + " as ",
	} {
		if index := strings.Index(line, marker); index >= 0 {
			fields := strings.Fields(line[index+len(marker):])
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
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
	supportsUser, helpErr := ghTokenUserSelectionSupported(ctx, runner)
	if helpErr != nil {
		return "", fmt.Errorf("inspect gh account-token support: %w", helpErr)
	}
	tokenArgs := []string{"auth", "token", "--hostname", host}
	if supportsUser {
		tokenArgs = append(tokenArgs, "--user", login)
	} else {
		if err := requireActiveLegacyGHAccount(ctx, runner, host, login); err != nil {
			return "", err
		}
	}
	raw, err := runner.Run(ctx, tokenArgs...)
	if err != nil {
		return "", fmt.Errorf("resolve gh account %s@%s: %w", login, host, err)
	}
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("resolve gh account %s@%s: empty token", login, host)
	}
	return token, nil
}

func ghTokenUserSelectionSupported(ctx context.Context, runner ghAccountCommandRunner) (bool, error) {
	if _, ok := runner.(systemGHAccountRunner); ok {
		return cachedGHTokenUserSupport()
	}
	help, err := runner.Run(ctx, "auth", "token", "--help")
	if err != nil {
		return false, err
	}
	return strings.Contains(help, "--user"), nil
}

func requireActiveLegacyGHAccount(
	ctx context.Context,
	runner ghAccountCommandRunner,
	host, login string,
) error {
	accounts, err := listGHAccounts(ctx, runner)
	if err != nil {
		return fmt.Errorf("verify active gh account %s@%s: %w", login, host, err)
	}
	for _, account := range accounts {
		if strings.EqualFold(account.Host, host) &&
			account.Login == login &&
			account.Active {
			return nil
		}
	}
	return fmt.Errorf(
		"gh CLI cannot select account %s@%s; make it active or upgrade gh",
		login,
		host,
	)
}
