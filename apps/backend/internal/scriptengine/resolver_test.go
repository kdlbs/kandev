package scriptengine

import (
	"fmt"
	"slices"
	"testing"
)

const literalTokenBranch = "topic/{{workspace.path}}"

type renderSamples struct {
	command   string
	mutations int
}

func sampleLiteralRenders(resolver *Resolver, template, want string) renderSamples {
	var samples renderSamples
	for range 128 {
		got := resolver.Resolve(template)
		if samples.command == "" {
			samples.command = got
		}
		if got != want {
			if samples.mutations == 0 {
				samples.command = got
			}
			samples.mutations++
		}
	}
	return samples
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.4
func TestResolverOriginalTemplateOnly(t *testing.T) {
	workspace := t.TempDir()
	resolver := NewResolver().
		WithProvider(RepositoryProvider(map[string]any{"base_branch": literalTokenBranch}, nil, nil, nil)).
		WithProvider(WorkspaceProvider(workspace))
	template := "git -C {{workspace.path}} checkout {{repository.branch}}\n"
	want := "git -C '" + workspace + "' checkout 'topic/{{workspace.path}}'\n"
	samples := sampleLiteralRenders(resolver, template, want)
	t.Logf("mutated renders: %d/128", samples.mutations)
	if samples.mutations != 0 {
		t.Fatalf("inserted branch was expanded: %q; want %q", samples.command, want)
	}
}

func TestResolverCompatibility(t *testing.T) {
	cases := []struct {
		name, template, want string
		resolver             *Resolver
	}{
		{"ordinary branch", "{{repository.branch}}", "'feature/login'",
			NewResolver().WithProvider(RepositoryProvider(map[string]any{"base_branch": "feature/login"}, nil, nil, nil))},
		{"repeated tokens", "{{x}}/{{x}}", "value/value", NewResolver().WithStatic(map[string]string{"x": "value"})},
		{"unknown literal", "{{missing}}/{{x}}", "{{missing}}/value", NewResolver().WithStatic(map[string]string{"x": "value"})},
		{"empty script", "", "", NewResolver().WithStatic(map[string]string{"x": "value"})},
		{"no vars", "{{x}}", "{{x}}", NewResolver()},
		{"empty value", "a{{x}}b", "ab", NewResolver().WithStatic(map[string]string{"x": ""})},
		{"later static", "{{x}}", "later", NewResolver().WithStatic(map[string]string{"x": "first"}).WithStatic(map[string]string{"x": "later"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.resolver.Resolve(tc.template); got != tc.want {
				t.Fatalf("Resolve() = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("provider precedence and evaluation", testResolverProviderPrecedence)
}

func testResolverProviderPrecedence(t *testing.T) {
	t.Helper()
	for _, staticLast := range []bool{false, true} {
		t.Run(fmt.Sprintf("staticLast=%t", staticLast), func(t *testing.T) {
			var calls []string
			provider := func(name string) PlaceholderProvider {
				return func() map[string]string {
					calls = append(calls, name)
					return map[string]string{"x": name}
				}
			}
			resolver := NewResolver()
			if !staticLast {
				resolver.WithStatic(map[string]string{"x": "static"})
			}
			resolver.WithProvider(provider("first")).WithProvider(provider("last"))
			if staticLast {
				resolver.WithStatic(map[string]string{"x": "static"})
			}
			if got := resolver.Resolve(""); got != "" || len(calls) != 0 {
				t.Fatalf("empty input = %q, provider calls = %v", got, calls)
			}
			if got := resolver.Resolve("{{x}}/{{x}}"); got != "last/last" {
				t.Fatalf("provider precedence = %q", got)
			}
			if !slices.Equal(calls, []string{"first", "last"}) {
				t.Fatalf("provider calls = %v", calls)
			}
		})
	}
}

func TestResolverScriptFragmentsVerbatim(t *testing.T) {
	fragment := "printf '%s\\n' '{{workspace.path}}' '{{unknown}}'\nprintf '%s' \"${VALUE:-fallback}\"\n"
	providers := []struct {
		name     string
		key      string
		provider PlaceholderProvider
	}{
		{"repository", "repository.setup_script", RepositoryProvider(map[string]any{"repository_setup_script": fragment}, nil, nil, nil)},
		{"agents", "kandev.agents.install", AgentInstallProvider([]string{fragment, fragment})},
		{"controller", "kandev.agentctl.start", AgentctlProvider(9999, "/workspace")},
		{"identity", "git.identity_setup", GitIdentityProvider(map[string]any{"git_user_name": "Example", "git_user_email": "example@example.test"})},
		{"authentication", "github.auth_setup", GitHubAuthProvider(map[string]string{"GH_TOKEN": "fixture-token"})},
	}
	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			want := "before\n" + tc.provider()[tc.key] + "\nafter"
			resolver := NewResolver().WithProvider(tc.provider).WithProvider(WorkspaceProvider("/different"))
			template := "before\n{{" + tc.key + "}}\nafter"
			samples := sampleLiteralRenders(resolver, template, want)
			if samples.mutations != 0 {
				t.Fatalf("fragment changed in %d/128 renders: %q, want %q", samples.mutations, samples.command, want)
			}
		})
	}
}
