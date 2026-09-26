package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func init() {
	if os.Getenv("KANDEV_GH_ACCOUNT_TEST_HELPER") != "1" {
		return
	}
	if len(os.Args) > 3 && os.Args[3] == "--json" {
		fmt.Fprintln(os.Stderr, "unknown flag: --json")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "github.com")
	fmt.Fprintln(os.Stderr, "  ✓ Logged in to github.com as alice (/home/alice/.config/gh/hosts.yml)")
	os.Exit(0)
}

type fakeGHAccountRunner struct {
	output string
	err    error
	args   [][]string
}

func (r *fakeGHAccountRunner) Run(_ context.Context, args ...string) (string, error) {
	r.args = append(r.args, append([]string(nil), args...))
	return r.output, r.err
}

func TestListGHAccountsParsesAllStoredLogins(t *testing.T) {
	runner := &fakeGHAccountRunner{output: `{"hosts":{"github.com":[{"active":true,"host":"github.com","login":"alice","state":"success"},{"active":false,"host":"github.com","login":"work-bot","state":"success"}]}}`}

	got, err := listGHAccounts(context.Background(), runner)
	if err != nil {
		t.Fatalf("listGHAccounts: %v", err)
	}
	want := []GHAccount{
		{Host: "github.com", Login: "alice", Active: true, State: "success"},
		{Host: "github.com", Login: "work-bot", State: "success"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("accounts = %#v, want %#v", got, want)
	}
	if wantArgs := []string{"auth", "status", "--json", "hosts"}; !reflect.DeepEqual(runner.args[0], wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args[0], wantArgs)
	}
}

func TestSystemGHAccountRunnerUsesStderrWhenStdoutEmpty(t *testing.T) {
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ghPath := filepath.Join(t.TempDir(), "gh")
	if runtime.GOOS == "windows" {
		ghPath += ".exe"
	}
	binary, err := os.ReadFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghPath, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(ghPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_GH_ACCOUNT_TEST_HELPER", "1")

	accounts, err := listGHAccounts(context.Background(), systemGHAccountRunner{})
	if err != nil {
		t.Fatalf("listGHAccounts() error = %v", err)
	}
	want := []GHAccount{{Host: "github.com", Login: "alice", Active: true, State: "success"}}
	if !reflect.DeepEqual(accounts, want) {
		t.Fatalf("accounts = %#v, want %#v", accounts, want)
	}
}

type legacyGHAccountRunner struct {
	output string
	args   [][]string
}

func (r *legacyGHAccountRunner) Run(_ context.Context, args ...string) (string, error) {
	r.args = append(r.args, append([]string(nil), args...))
	if len(args) == 4 && args[2] == "--json" {
		return "", errors.New("unknown flag: --json")
	}
	return r.output, nil
}

func TestListGHAccountsSupportsLegacyStatusOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []GHAccount
	}{
		{
			name: "multi-account CLI without JSON output",
			output: `github.com
  ✓ Logged in to github.com account alice (/home/alice/.config/gh/hosts.yml)
  - Active account: true
  - Git operations protocol: ssh

  ✓ Logged in to github.com account work-bot (/home/alice/.config/gh/hosts.yml)
  - Active account: false
  - Git operations protocol: https
`,
			want: []GHAccount{
				{Host: "github.com", Login: "alice", Active: true, State: "success"},
				{Host: "github.com", Login: "work-bot", State: "success"},
			},
		},
		{
			name: "single-account CLI before account switching",
			output: `github.com
  ✓ Logged in to github.com as alice (/home/alice/.config/gh/hosts.yml)
  ✓ Git operations for github.com configured to use ssh protocol.
`,
			want: []GHAccount{
				{Host: "github.com", Login: "alice", Active: true, State: "success"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &legacyGHAccountRunner{output: tt.output}
			got, err := listGHAccounts(context.Background(), runner)
			if err != nil {
				t.Fatalf("listGHAccounts: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("accounts = %#v, want %#v", got, tt.want)
			}
			wantCalls := [][]string{
				{"auth", "status", "--json", "hosts"},
				{"auth", "status"},
			}
			if !reflect.DeepEqual(runner.args, wantCalls) {
				t.Fatalf("args = %#v, want %#v", runner.args, wantCalls)
			}
		})
	}
}

func TestListGHAccountsRejectsUnrecognizedLegacyStatusOutput(t *testing.T) {
	runner := &legacyGHAccountRunner{output: "authentication state unavailable"}
	accounts, err := listGHAccounts(context.Background(), runner)
	if err == nil {
		t.Fatalf("listGHAccounts() = %#v, want parse error", accounts)
	}
	if strings.Contains(err.Error(), runner.output) {
		t.Fatalf("error exposed raw gh auth output: %v", err)
	}
}

type modernGHTokenRunner struct {
	tokenErr error
	args     [][]string
}

func (r *modernGHTokenRunner) Run(_ context.Context, args ...string) (string, error) {
	r.args = append(r.args, append([]string(nil), args...))
	if len(args) == 3 && args[2] == "--help" {
		return "  -u, --user string   The account to output the token for\n", nil
	}
	if r.tokenErr != nil {
		return "", r.tokenErr
	}
	return "secret-token\n", nil
}

func TestResolveGHAccountTokenSelectsExactAccount(t *testing.T) {
	runner := &modernGHTokenRunner{}
	token, err := resolveGHAccountToken(context.Background(), runner, "github.com", "work-bot")
	if err != nil {
		t.Fatalf("resolveGHAccountToken: %v", err)
	}
	if token != "secret-token" {
		t.Fatalf("token = %q", token)
	}
	wantCalls := [][]string{
		{"auth", "token", "--help"},
		{"auth", "token", "--hostname", "github.com", "--user", "work-bot"},
	}
	if !reflect.DeepEqual(runner.args, wantCalls) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantCalls)
	}
}

type legacyGHTokenRunner struct {
	args [][]string
}

func (r *legacyGHTokenRunner) Run(_ context.Context, args ...string) (string, error) {
	r.args = append(r.args, append([]string(nil), args...))
	if len(args) == 3 && args[2] == "--help" {
		return "  -h, --hostname string   The hostname to output the token for\n", nil
	}
	if len(args) == 4 && args[2] == "--json" {
		return "", errors.New("unknown flag: --json")
	}
	if len(args) == 2 && args[0] == "auth" && args[1] == "status" {
		return `github.com
  ✓ Logged in to github.com as alice (/home/alice/.config/gh/hosts.yml)
`, nil
	}
	if len(args) == 6 && args[4] == "--user" {
		return "", errors.New("legacy CLI must not receive --user")
	}
	return "legacy-secret-token\n", nil
}

func TestResolveGHAccountTokenSupportsLegacySingleAccountCLI(t *testing.T) {
	runner := &legacyGHTokenRunner{}
	token, err := resolveGHAccountToken(context.Background(), runner, "github.com", "alice")
	if err != nil {
		t.Fatalf("resolveGHAccountToken: %v", err)
	}
	if token != "legacy-secret-token" {
		t.Fatalf("token = %q", token)
	}
	wantCalls := [][]string{
		{"auth", "token", "--help"},
		{"auth", "status", "--json", "hosts"},
		{"auth", "status"},
		{"auth", "token", "--hostname", "github.com"},
	}
	if !reflect.DeepEqual(runner.args, wantCalls) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantCalls)
	}
}

func TestResolveGHAccountTokenRejectsInactiveAccountOnLegacyCLI(t *testing.T) {
	runner := &legacyInactiveGHTokenRunner{}
	_, err := resolveGHAccountToken(context.Background(), runner, "github.com", "work-bot")
	if err == nil || !strings.Contains(err.Error(), "make it active or upgrade gh") {
		t.Fatalf("resolveGHAccountToken() error = %v", err)
	}
	for _, args := range runner.args {
		if reflect.DeepEqual(args, []string{"auth", "token", "--hostname", "github.com"}) {
			t.Fatal("legacy CLI token requested for an inactive account")
		}
	}
}

type legacyInactiveGHTokenRunner struct {
	args [][]string
}

func (r *legacyInactiveGHTokenRunner) Run(_ context.Context, args ...string) (string, error) {
	r.args = append(r.args, append([]string(nil), args...))
	switch {
	case len(args) == 3 && args[2] == "--help":
		return "  -h, --hostname string\n", nil
	case len(args) == 4 && args[2] == "--json":
		return "", errors.New("unknown flag: --json")
	case len(args) == 2 && args[0] == "auth" && args[1] == "status":
		return `github.com
  ✓ Logged in to github.com account alice (/home/alice/.config/gh/hosts.yml)
  - Active account: true

  ✓ Logged in to github.com account work-bot (/home/alice/.config/gh/hosts.yml)
  - Active account: false
`, nil
	default:
		return "must-not-return-token", nil
	}
}

func TestResolveGHAccountTokenDoesNotFallBackAfterNamedAccountFailure(t *testing.T) {
	runner := &modernGHTokenRunner{tokenErr: errors.New("account token unavailable")}
	if _, err := resolveGHAccountToken(context.Background(), runner, "github.com", "work-bot"); err == nil {
		t.Fatal("expected named account token failure")
	}
	wantCalls := [][]string{
		{"auth", "token", "--help"},
		{"auth", "token", "--hostname", "github.com", "--user", "work-bot"},
	}
	if !reflect.DeepEqual(runner.args, wantCalls) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantCalls)
	}
}

func TestResolveGHAccountTokenRejectsUnsupportedHost(t *testing.T) {
	runner := &fakeGHAccountRunner{output: "must-not-run"}
	if _, err := resolveGHAccountToken(context.Background(), runner, "git.example.com", "alice"); err == nil {
		t.Fatal("expected unsupported host error")
	}
	if len(runner.args) != 0 {
		t.Fatalf("runner called with %#v", runner.args)
	}
}

func TestGitHubCLIEnvironmentStripsAmbientTokens(t *testing.T) {
	got := githubCLIEnvironment([]string{
		"PATH=/bin",
		"GH_TOKEN=gh-secret",
		"github_token=github-secret",
		"HOME=/tmp/home",
	})
	want := []string{"PATH=/bin", "HOME=/tmp/home"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %#v, want %#v", got, want)
	}
	if strings.Contains(strings.Join(got, "\n"), "secret") {
		t.Fatal("filtered environment contains token material")
	}
}
