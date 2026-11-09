package lifecycle

import (
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/startupdiag"
	"github.com/kandev/kandev/internal/task/models"
)

// startupProcessBootstrapFailure converts matching pre-initialize process
// evidence into the existing bootstrap cause contract. Evidence must describe
// the same generation captured for this initialization and an ordinary exit.
func startupProcessBootstrapFailure(execution *AgentExecution, err error) *BootstrapFailure {
	var phaseErr *SessionInitializationPhaseError
	if !errors.As(err, &phaseErr) || phaseErr == nil ||
		phaseErr.Phase != SessionInitializationPhaseACPInitialize {
		return nil
	}
	var initializeErr *agentctl.InitializeError
	if !errors.As(err, &initializeErr) || initializeErr == nil {
		return nil
	}
	evidence := initializeErr.StartupEvidence
	if !startupProcessEvidenceMatches(phaseErr.ProcessGeneration, evidence) {
		return nil
	}
	exitCode := *evidence.ExitCode
	reason := models.AgentErrorCauseReasonUnclassified
	if evidence.DiagnosticComplete &&
		evidence.DiagnosticReason == startupdiag.ReasonMiseConfigUntrusted {
		reason = models.AgentErrorCauseReasonMiseConfigUntrusted
	}
	return &BootstrapFailure{
		Operation: bootstrapOperation(execution),
		Code:      models.AgentErrorCauseCodeProcessExited,
		Reason:    reason,
		ExitCode:  &exitCode,
		Cause:     err,
	}
}

func startupProcessEvidenceMatches(
	generation uint64,
	evidence *agentctltypes.ManagedStartupEvidence,
) bool {
	if evidence == nil || generation == 0 || evidence.ProcessGeneration != generation {
		return false
	}
	if evidence.ExitDisposition != agentctltypes.ManagedStartupExitOrdinary {
		return false
	}
	return evidence.ExitCode != nil && *evidence.ExitCode >= 0
}
