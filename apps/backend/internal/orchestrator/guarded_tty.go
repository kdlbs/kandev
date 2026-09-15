package orchestrator

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

const guardedTTYFinalizeTimeout = 5 * time.Second

var (
	ErrGuardedTTYUnavailable = errors.New("guarded TTY execution is unavailable")
	ErrGuardedTTYAudit       = errors.New("guarded TTY execution audit failed")
	ErrGuardedTTYProvider    = errors.New("guarded TTY provider receipt is invalid")
)

type guardedTTYAgentExecutor interface {
	ExecuteGuardedTTY(context.Context, models.GuardedTTYDispatchRequest) (*streams.GuardedTTYExecReceipt, error)
}

type guardedTTYAuditRecorder interface {
	ClaimGuardedTTYExecution(context.Context, models.GuardedTTYAuditClaim) error
	FinalizeGuardedTTYExecution(context.Context, models.GuardedTTYAuditFinalize) error
}

// ExecuteGuardedTTY serializes durable evidence before dispatch, calls only
// the current lifecycle execution, validates the provider receipt, and closes
// the same attestation without persisting raw output.
func (s *Service) ExecuteGuardedTTY(ctx context.Context, request streams.GuardedTTYExecRequest) (*streams.GuardedTTYExecReceipt, error) {
	principal, executor, audit, ok := s.guardedTTYDependencies(ctx, request.Execution)
	if !ok {
		return nil, ErrGuardedTTYUnavailable
	}
	attestationID := uuid.NewString()
	actorUserID := ""
	if identity, hasIdentity := authn.IdentityFromContext(ctx); hasIdentity {
		actorUserID = identity.UserID
	}
	claim := models.GuardedTTYAuditClaim{
		AttestationID:    attestationID,
		Execution:        request.Execution,
		WorkspaceID:      principal.WorkspaceID,
		ActorUserID:      actorUserID,
		AgentID:          streams.GuardedTTYAgentID,
		PrincipalSurface: string(principal.Surface),
		Argv:             append([]string(nil), request.Argv...),
		RequestedAt:      nowUTC(),
		Outcome:          models.GuardedTTYOutcomePending,
	}
	if err := audit.ClaimGuardedTTYExecution(ctx, claim); err != nil {
		return nil, fmt.Errorf("%w: claim", ErrGuardedTTYAudit)
	}
	receipt, dispatchErr := executor.ExecuteGuardedTTY(ctx, models.GuardedTTYDispatchRequest{
		AttestationID: attestationID,
		Execution:     request.Execution,
		Argv:          append([]string(nil), request.Argv...),
	})
	if dispatchErr != nil {
		finalize := failedGuardedTTYFinalization(attestationID, request.Execution, models.GuardedTTYOutcomeProviderFailure)
		if err := finalizeGuardedTTYAudit(ctx, audit, finalize); err != nil {
			return nil, fmt.Errorf("%w: finalize provider failure", ErrGuardedTTYAudit)
		}
		return nil, ErrGuardedTTYProvider
	}
	outcome, err := validateGuardedTTYReceipt(request.Execution, receipt)
	if err != nil {
		finalize := failedGuardedTTYFinalization(attestationID, request.Execution, models.GuardedTTYOutcomeProviderFailure)
		if finalizeErr := finalizeGuardedTTYAudit(ctx, audit, finalize); finalizeErr != nil {
			return nil, fmt.Errorf("%w: finalize invalid receipt", ErrGuardedTTYAudit)
		}
		return nil, fmt.Errorf("%w: %v", ErrGuardedTTYProvider, err)
	}

	receipt.AttestationID = attestationID
	receipt.OutputBytes = len(receipt.Output)
	receipt.OutputSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(receipt.Output)))
	if err := finalizeGuardedTTYAudit(ctx, audit, completedGuardedTTYFinalization(attestationID, request.Execution, receipt, outcome)); err != nil {
		return nil, fmt.Errorf("%w: finalize receipt", ErrGuardedTTYAudit)
	}
	if outcome != models.GuardedTTYOutcomeSucceeded {
		return nil, ErrGuardedTTYProvider
	}
	return receipt, nil
}

func (s *Service) guardedTTYDependencies(
	ctx context.Context,
	execution streams.MCPExecutionContext,
) (mcpscope.Principal, guardedTTYAgentExecutor, guardedTTYAuditRecorder, bool) {
	principal, principalOK := mcpscope.PrincipalFromContext(ctx)
	if !principalOK || principal.Surface != mcpprofile.SurfaceKanbanTask || principal.WorkspaceID == "" ||
		principal.CallerTaskID != execution.TaskID || principal.CallerSessionID != execution.SessionID {
		return mcpscope.Principal{}, nil, nil, false
	}
	executor, executorOK := s.agentManager.(guardedTTYAgentExecutor)
	audit, auditOK := s.messageCreator.(guardedTTYAuditRecorder)
	return principal, executor, audit, executorOK && auditOK
}

func completedGuardedTTYFinalization(
	attestationID string,
	execution streams.MCPExecutionContext,
	receipt *streams.GuardedTTYExecReceipt,
	outcome models.GuardedTTYOutcome,
) models.GuardedTTYAuditFinalize {
	return models.GuardedTTYAuditFinalize{
		AttestationID:   attestationID,
		Execution:       execution,
		Outcome:         outcome,
		ExitCode:        receipt.ExitCode,
		OutputBytes:     receipt.OutputBytes,
		OutputSHA256:    receipt.OutputSHA256,
		CompletionCount: receipt.CompletionCount,
		CompletedAt:     receipt.CompletedAt,
		ProviderMetadata: map[string]interface{}{
			"contract_version": receipt.ContractVersion,
			"bridge_version":   receipt.BridgeVersion,
			"method":           receipt.Method,
			"requested_tty":    receipt.RequestedTTY,
			"dispatched_tty":   receipt.DispatchedTTY,
			"process_id":       receipt.ProcessID,
			"cwd":              receipt.CWD,
			"started_at":       receipt.StartedAt,
		},
	}
}

func finalizeGuardedTTYAudit(
	ctx context.Context,
	audit guardedTTYAuditRecorder,
	finalize models.GuardedTTYAuditFinalize,
) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), guardedTTYFinalizeTimeout)
	defer cancel()
	return audit.FinalizeGuardedTTYExecution(writeCtx, finalize)
}

func validateGuardedTTYReceipt(
	execution streams.MCPExecutionContext,
	receipt *streams.GuardedTTYExecReceipt,
) (models.GuardedTTYOutcome, error) {
	if err := validateGuardedTTYReceiptEvidence(execution, receipt); err != nil {
		return "", err
	}
	if receipt.Outcome == string(models.GuardedTTYOutcomeSucceeded) {
		if !receipt.DispatchedTTY || receipt.ProcessID == "" || receipt.CWD == "" || receipt.DenialCode != "" {
			return "", errors.New("successful receipt lacks TTY dispatch evidence")
		}
		return models.GuardedTTYOutcomeSucceeded, nil
	}
	if err := validateGuardedTTYDenialEvidence(receipt); err != nil {
		return "", err
	}
	switch receipt.DenialCode {
	case streams.GuardedTTYDenialStale:
		return models.GuardedTTYOutcomeStale, nil
	case streams.GuardedTTYDenialTimeout:
		return models.GuardedTTYOutcomeTimedOut, nil
	case streams.GuardedTTYDenialCancelled:
		return models.GuardedTTYOutcomeCancelled, nil
	case streams.GuardedTTYDenialOverflow:
		return models.GuardedTTYOutcomeOverflow, nil
	case streams.GuardedTTYDenialInvalid, streams.GuardedTTYDenialAppServer:
		return models.GuardedTTYOutcomeProviderFailure, nil
	default:
		return "", errors.New("receipt denial code is unknown")
	}
}

func validateGuardedTTYReceiptEvidence(
	execution streams.MCPExecutionContext,
	receipt *streams.GuardedTTYExecReceipt,
) error {
	if receipt == nil {
		return errors.New("receipt is missing")
	}
	if err := validateGuardedTTYReceiptIdentity(execution, receipt); err != nil {
		return err
	}
	if err := validateGuardedTTYReceiptTiming(receipt); err != nil {
		return err
	}
	return validateGuardedTTYReceiptOutput(receipt)
}

func validateGuardedTTYReceiptIdentity(
	execution streams.MCPExecutionContext,
	receipt *streams.GuardedTTYExecReceipt,
) error {
	if receipt.ExecutionID != execution.ExecutionID || receipt.TaskID != execution.TaskID || receipt.SessionID != execution.SessionID {
		return errors.New("receipt execution identity mismatch")
	}
	if receipt.ContractVersion != streams.GuardedTTYContractVersion || receipt.BridgeVersion == "" || receipt.ACPSessionID == "" {
		return errors.New("receipt bridge identity is incomplete")
	}
	if receipt.Method != streams.GuardedTTYExecMethod || !receipt.RequestedTTY {
		return errors.New("receipt does not prove a TTY command/exec request")
	}
	return nil
}

func validateGuardedTTYReceiptTiming(receipt *streams.GuardedTTYExecReceipt) error {
	if receipt.CompletionCount != 1 || receipt.StartedAt.IsZero() || receipt.CompletedAt.IsZero() ||
		receipt.CompletedAt.Before(receipt.StartedAt) {
		return errors.New("receipt does not contain exactly one completion")
	}
	return nil
}

func validateGuardedTTYReceiptOutput(receipt *streams.GuardedTTYExecReceipt) error {
	if receipt.StdoutBytes != len(receipt.Stdout) || receipt.StderrBytes != len(receipt.Stderr) ||
		receipt.Output != receipt.Stdout+receipt.Stderr || receipt.OutputBytes != len(receipt.Output) ||
		len(receipt.Output) > streams.GuardedTTYMaxOutputBytes {
		return errors.New("receipt output size is invalid")
	}
	return nil
}

func validateGuardedTTYDenialEvidence(receipt *streams.GuardedTTYExecReceipt) error {
	if receipt.Outcome == "" || receipt.Outcome != receipt.DenialCode {
		return errors.New("receipt denial outcome is inconsistent")
	}
	dispatchIdentity := receipt.ProcessID != "" && receipt.CWD != ""
	if receipt.DenialCode == streams.GuardedTTYDenialStale {
		if receipt.DispatchedTTY || receipt.ProcessID != "" || receipt.CWD != "" {
			return errors.New("stale receipt contains dispatch evidence")
		}
		return nil
	}
	if !receipt.DispatchedTTY || !dispatchIdentity {
		return errors.New("undispatched receipt has an invalid denial")
	}
	return nil
}

func failedGuardedTTYFinalization(attestationID string, execution streams.MCPExecutionContext, outcome models.GuardedTTYOutcome) models.GuardedTTYAuditFinalize {
	return models.GuardedTTYAuditFinalize{
		AttestationID: attestationID,
		Execution:     execution,
		Outcome:       outcome,
		CompletedAt:   nowUTC(),
		ProviderMetadata: map[string]interface{}{
			"method":         streams.GuardedTTYExecMethod,
			"requested_tty":  true,
			"dispatched_tty": nil,
		},
	}
}

var nowUTC = func() time.Time { return time.Now().UTC() }
