package managedruntime

import (
	"reflect"
	"testing"
)

func TestManagedNPMInvocation(t *testing.T) {
	packageName := "@agentclientprotocol/claude-agent-acp"
	packageSpec := packageName + "@0.100.0"
	required := []string{}
	args := []string{
		"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec,
		"--dangerously-skip-permissions", packageSpec, "--cache", "/agent/value",
	}
	invocation, err := ParseManagedNPMInvocation(args, packageName, required)
	if err != nil {
		t.Fatalf("parse valid invocation: %v", err)
	}
	if invocation.PackageIndex != 5 || invocation.PreferenceIndex != 2 || invocation.PackageSpec != packageSpec {
		t.Fatalf("invocation positions = %+v, want package at 5 and preference at 2", invocation)
	}
	if got, want := invocation.AgentArgs(), args[6:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("agent args = %#v, want %#v", got, want)
	}

	online := invocation.WithNpmPreference(true)
	if online[2] != "--prefer-online" {
		t.Fatalf("online preference = %q, want --prefer-online", online[2])
	}
	online[6] = "changed"
	if args[6] != "--dangerously-skip-permissions" {
		t.Fatal("preference rewrite mutated the original command")
	}

	codex := []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, "@openai/codex@1.2.3", "app-server", "--profile-flag", "value"}
	codexInvocation, err := ParseManagedNPMInvocation(codex, "@openai/codex", []string{"app-server"})
	if err != nil {
		t.Fatalf("parse required runtime argument: %v", err)
	}
	if got, want := codexInvocation.AgentArgs(), []string{"--profile-flag", "value"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("codex agent args = %#v, want %#v", got, want)
	}
}

func TestManagedNPMInvocationRejectsUntrustedShapes(t *testing.T) {
	packageName := "@scope/managed-acp"
	packageSpec := packageName + "@1.2.3"
	valid := func() []string {
		return []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec, "--acp"}
	}
	cases := []struct {
		name        string
		packageName string
		args        []string
		req         []string
	}{
		{name: "wrapper prefix", packageName: packageName, args: append([]string{"wrapper"}, valid()...)},
		{name: "shell prefix", packageName: packageName, args: append([]string{"sh", "-c"}, valid()...)},
		{name: "missing yes", packageName: packageName, args: []string{"npx", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec, "--acp"}},
		{name: "unknown npm option", packageName: packageName, args: []string{"npx", "--yes", "--prefer-offline", "--cache", NPMProjectPrefix, packageSpec, "--acp"}},
		{name: "duplicate prefix", packageName: packageName, args: []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, "--prefix", NPMProjectPrefix, packageSpec, "--acp"}},
		{name: "wrong prefix marker", packageName: packageName, args: []string{"npx", "--yes", "--prefer-offline", "--prefix", "/tmp/cache", packageSpec, "--acp"}},
		{name: "wrong package", packageName: packageName, args: []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, "@scope/other@1.2.3", "--acp"}},
		{name: "inexact package", packageName: packageName, args: []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageName, "--acp"}},
		{name: "missing required runtime argument", packageName: packageName, args: valid(), req: []string{"app-server"}},
		{name: "wrong required runtime argument", packageName: packageName, args: []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec, "acp"}, req: []string{"app-server"}},
		{name: "preference after package", packageName: packageName, args: []string{"npx", "--yes", packageSpec, "--prefer-offline", "--prefix", NPMProjectPrefix, "--acp"}},
		{name: "empty package name", packageName: "", args: []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec, "--acp"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseManagedNPMInvocation(tt.args, tt.packageName, tt.req); err == nil {
				t.Fatalf("ParseManagedNPMInvocation(%#v) succeeded, want rejection", tt.args)
			}
		})
	}
}
