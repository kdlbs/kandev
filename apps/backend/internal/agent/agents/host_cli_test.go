package agents

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostcli"
)

func TestBuiltInHostCLISpecs(t *testing.T) {
	claude := NewClaudeACP().HostCLI()
	if claude.Executable != "claude" || claude.HasModelSource() {
		t.Fatalf("claude spec = %+v", claude)
	}
	codex := NewCodexACP().HostCLI()
	if codex.Executable != "codex" || codex.ModelSource != hostcli.ModelSourceCodexAppServer {
		t.Fatalf("codex spec = %+v", codex)
	}
	for _, spec := range []hostcli.Spec{claude, codex} {
		if !spec.Valid() || len(spec.VersionArgs) == 0 {
			t.Fatalf("spec %s is incomplete: %+v", spec.DisplayName, spec)
		}
	}
	var _ HostCLIAgent = NewClaudeACP()
	var _ HostCLIAgent = NewCodexACP()
}

func TestMockAgentHostCLIUsesBinaryPath(t *testing.T) {
	mock := NewMockAgent()
	mock.SetBinaryPath("/tmp/bin/mock-agent")
	spec := mock.HostCLI()
	if spec.Executable != "/tmp/bin/mock-agent" || spec.ModelSource != hostcli.ModelSourceCodexAppServer {
		t.Fatalf("mock spec = %+v", spec)
	}
	result, err := mock.IsInstalled(context.Background())
	if err != nil || result.MatchedPath != "/tmp/bin/mock-agent" {
		t.Fatalf("IsInstalled = %+v, %v", result, err)
	}
}
