package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/worktree"
)

func TestPassthroughAgentCommandRejectsGitMetadataPolicy(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	execution.GitMetadataProjections = []*worktree.GitMetadataProjection{{CheckoutPath: execution.WorkspacePath}}

	_, _, _, _, err := mgr.passthroughAgentCommand(context.Background(), execution, profile)

	if err == nil || !strings.Contains(err.Error(), "cannot enforce task Git metadata permissions in passthrough mode") {
		t.Fatalf("passthroughAgentCommand error = %v, want explicit policy-enforcement error", err)
	}
}
