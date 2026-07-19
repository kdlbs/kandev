package github

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

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

func TestResolveGHAccountTokenSelectsExactAccount(t *testing.T) {
	runner := &fakeGHAccountRunner{output: "secret-token\n"}

	token, err := resolveGHAccountToken(context.Background(), runner, "github.com", "work-bot")
	if err != nil {
		t.Fatalf("resolveGHAccountToken: %v", err)
	}
	if token != "secret-token" {
		t.Fatalf("token = %q", token)
	}
	wantArgs := []string{"auth", "token", "--hostname", "github.com", "--user", "work-bot"}
	if !reflect.DeepEqual(runner.args[0], wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args[0], wantArgs)
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
