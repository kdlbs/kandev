package mcpconfig

import (
	"reflect"
	"testing"
)

func TestManagedPackageCommandUsesScopedPackageExecutableAndArguments(t *testing.T) {
	command, args, err := ManagedPackageCommand(MCPServerConfiguration{
		PackageType:             "npm",
		PackageName:             "@example/mcp-server",
		PackageVersion:          "1.2.3",
		PackageRegistry:         "https://registry.example.test",
		PackageExecutable:       "mcp-server",
		PackageRuntimeArguments: []string{"--runtime-flag"},
		PackageArguments:        []string{"--stdio"},
	})
	if err != nil {
		t.Fatalf("ManagedPackageCommand: %v", err)
	}
	wantArgs := []string{
		"--yes", "--package", "@example/mcp-server@1.2.3",
		"--registry", "https://registry.example.test", "--runtime-flag",
		"mcp-server", "--stdio",
	}
	if command != "npx" || !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("managed command = %q %#v, want npx %#v", command, args, wantArgs)
	}
}

func TestManagedPackageCommandFallsBackToScopedPackageBaseName(t *testing.T) {
	_, args, err := ManagedPackageCommand(MCPServerConfiguration{
		PackageType:    "npm",
		PackageName:    "@example/mcp-server",
		PackageVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("ManagedPackageCommand: %v", err)
	}
	if got := args[len(args)-1]; got != "mcp-server" {
		t.Fatalf("fallback executable = %q, want mcp-server", got)
	}
}
