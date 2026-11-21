package executor

import (
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestBootstrapFailureCauseProcessExitStatus(t *testing.T) {
	exitCode := 1
	causes, safeReason := bootstrapFailureCause(&agentruntime.BootstrapFailure{
		Operation: models.AgentErrorCauseOperationResume,
		Code:      models.AgentErrorCauseCodeProcessExited,
		Reason:    models.AgentErrorCauseReasonMiseConfigUntrusted,
		ExitCode:  &exitCode,
	}, true)
	if safeReason != models.AgentErrorCauseCodeProcessExited || len(causes) != 1 {
		t.Fatalf("causes = %#v, safeReason = %q", causes, safeReason)
	}
	cause := causes[0]
	if cause.ExitCode == nil || *cause.ExitCode != 1 ||
		cause.Reason != models.AgentErrorCauseReasonMiseConfigUntrusted ||
		cause.Detail != models.AgentErrorCauseProcessExitedDetail {
		t.Fatalf("process exit cause = %#v", cause)
	}
}

func TestBootstrapFailureCauseDoesNotTrustProcessDetail(t *testing.T) {
	exitCode := 2
	causes, _ := bootstrapFailureCause(&agentruntime.BootstrapFailure{
		Operation: models.AgentErrorCauseOperationStart,
		Code:      models.AgentErrorCauseCodeProcessExited,
		Reason:    "private_reason",
		Detail:    "raw /private/workspace/mise.toml",
		ExitCode:  &exitCode,
	}, false)
	if len(causes) != 1 {
		t.Fatalf("causes = %#v", causes)
	}
	cause := causes[0]
	if cause.Detail != models.AgentErrorCauseProcessExitedDetail ||
		cause.Reason != models.AgentErrorCauseReasonUnclassified {
		t.Fatalf("cause = %#v, want fixed detail and unclassified reason", cause)
	}
}
