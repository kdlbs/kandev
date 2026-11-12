package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedRuntimeCacheEvidence(t *testing.T) {
	cacheRoot := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	executionRoot := filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
	packageManifest := filepath.Join(executionRoot, "node_modules", "@agentclientprotocol", "claude-agent-acp", "package.json")
	if err := os.MkdirAll(filepath.Dir(packageManifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageManifest, []byte(`{"name":"@agentclientprotocol/claude-agent-acp","version":"1.2.3","bin":{"claude-agent-acp":"bin/cli.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "package.json")
	failure := &types.ManagedStartupEvidence{
		ProcessGeneration: 17, ExitDisposition: types.ManagedStartupExitOrdinary, ExitCode: ptrInt(254),
		CollectionComplete: true, NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
	}
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	got := managedRuntimeCacheEvidenceForFailure(cacheRoot, "claude-acp", command, failure, stderr)
	if got == nil || got.ProcessGeneration != failure.ProcessGeneration ||
		got.PackageSpec != packageSpec || got.Condition != types.ManagedRuntimeCacheRootManifestMissing {
		t.Fatalf("managed runtime cache evidence = %#v, want exact generation/package and root manifest condition", got)
	}

	for name, test := range map[string]struct {
		command []string
		failure *types.ManagedStartupEvidence
		stderr  string
	}{
		"wrong diagnostic path":            {command: command, failure: failure, stderr: "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "other.json")},
		"permanent error takes precedence": {command: command, failure: failure, stderr: "npm error code EACCES\nnpm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "package.json")},
		"noncanonical managed command":     {command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "operator-project", packageSpec}, failure: failure, stderr: stderr},
		"intentional stop":                 {command: command, failure: &types.ManagedStartupEvidence{ProcessGeneration: 18, ExitDisposition: types.ManagedStartupExitIntentional, CollectionComplete: true, NPMDiagnosticComplete: true}, stderr: stderr},
		"incomplete diagnostics":           {command: command, failure: &types.ManagedStartupEvidence{ProcessGeneration: 18, ExitDisposition: types.ManagedStartupExitOrdinary, CollectionComplete: true, NPMDiagnosticComplete: false}, stderr: stderr},
	} {
		t.Run(name, func(t *testing.T) {
			if evidence := managedRuntimeCacheEvidenceForFailure(cacheRoot, "claude-acp", test.command, test.failure, test.stderr); evidence != nil {
				t.Fatalf("cache evidence = %#v, want fail-closed rejection", evidence)
			}
		})
	}
}

func TestManagedRuntimeCacheEvidenceWithCLIArgs(t *testing.T) {
	cacheRoot := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@0.100.0"
	executionRoot := filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
	packageManifest := filepath.Join(executionRoot, "node_modules", "@agentclientprotocol", "claude-agent-acp", "package.json")
	if err := os.MkdirAll(filepath.Dir(packageManifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageManifest, []byte(`{"name":"@agentclientprotocol/claude-agent-acp","version":"0.100.0","bin":{"claude-agent-acp":"bin/cli.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "package.json")
	failure := &types.ManagedStartupEvidence{
		ProcessGeneration: 17, ExitDisposition: types.ManagedStartupExitOrdinary, ExitCode: ptrInt(254),
		CollectionComplete: true, NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
	}
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("0.100.0").Args()
	command = append(command, "--dangerously-skip-permissions")
	got := managedRuntimeCacheEvidenceForFailure(cacheRoot, "claude-acp", command, failure, stderr)
	if got == nil || got.ProcessGeneration != failure.ProcessGeneration ||
		got.PackageSpec != packageSpec || got.Condition != types.ManagedRuntimeCacheRootManifestMissing {
		t.Fatalf("flagged command evidence = %#v, want exact generation/package and root manifest condition", got)
	}
	opaque := append(append([]string(nil), command[:5]...), packageSpec+":agent-value")
	if evidence := managedRuntimeCacheEvidenceForFailure(cacheRoot, "claude-acp", opaque, failure, stderr); evidence != nil {
		t.Fatalf("package-like agent value produced installation authority: %#v", evidence)
	}
}

func ptrInt(value int) *int { return &value }

func TestManagedRuntimeCacheOpenCodeV2Command(t *testing.T) {
	spec, err := agents.NewOpenCodeACP().ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		t.Fatal(err)
	}
	packageSpec, ok := exactManagedRuntimePackage(agents.OpenCodeACPAgentID, spec.RuntimeCommand("2.0.1").Args())
	if !ok || packageSpec != "@opencode/cli@2.0.1" {
		t.Fatalf("registered selected runtime = %q, %v; want OpenCode v2", packageSpec, ok)
	}
}
