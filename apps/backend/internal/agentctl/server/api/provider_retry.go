package api

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// verifyFailedProviderRetry proves that the candidate never acquired prompt
// admission. Only this closed candidate can be replaced by an explicit retry.
func (s *Server) verifyFailedProviderRetry(ctx context.Context, storage *journal.Journal, previousID string, expected types.ProviderReplacementSource) error {
	previous, err := storage.ReadProviderReplacement(ctx, previousID)
	if err != nil {
		return err
	}
	if previous.Phase == journal.ProviderReplacementReleased {
		if replacementReceiptSource(previous) != expected {
			return journal.ErrProviderReplacementConflict
		}
		return s.verifyProviderReplacementSource(ctx, expected)
	}
	if !providerRetryPhaseAdmissible(previous) {
		return journal.ErrProviderReplacementConflict
	}
	current, err := s.procMgr.ProviderReplacementSource()
	if err != nil {
		return err
	}
	current.NativeSessionID = previous.NativeSessionID
	if current != expected || !providerRetryOwnerMatches(current, previous) || !providerRetryScopeMatches(current, previous) {
		return journal.ErrProviderReplacementConflict
	}
	status := s.procMgr.Status()
	if status != process.StatusRunning && status != process.StatusStopped && status != process.StatusError {
		return journal.ErrProviderReplacementConflict
	}
	if previous.Phase != journal.ProviderReplacementAccepted {
		return nil
	}
	if err := s.verifyAcceptedRetryDelivery(ctx, expected); err != nil {
		return err
	}
	if status == process.StatusRunning {
		return s.verifyProviderReplacementSource(ctx, expected)
	}
	return nil
}

func providerRetryPhaseAdmissible(previous journal.ProviderReplacementReceipt) bool {
	return (previous.Phase == journal.ProviderReplacementFailed || previous.Phase == journal.ProviderReplacementAccepted) && (previous.SourceTerminated || previous.PreviousOperationID != "")
}
func providerRetryOwnerMatches(current types.ProviderReplacementSource, previous journal.ProviderReplacementReceipt) bool {
	return current.StopGeneration == previous.SourceStopGeneration && current.ExecutionID == previous.ExecutionID && current.SessionID == previous.SessionID && current.TaskID == previous.SourceTaskID && current.HelperID == previous.HelperID && current.HarnessGeneration == previous.SourceHarnessGeneration && current.StreamID == previous.SourceStreamID && current.IncarnationID == previous.SourceIncarnationID
}
func providerRetryScopeMatches(current types.ProviderReplacementSource, previous journal.ProviderReplacementReceipt) bool {
	if previous.TargetScope.BindingID != "" {
		return current.BindingID == previous.TargetScope.BindingID && current.ProcessGeneration == previous.TargetScope.ProcessGeneration && current.ProfileDigest == previous.TargetScope.ProfileDigest
	}
	return current.BindingID == previous.SourceBindingID && current.ProcessGeneration == previous.SourceProcessGeneration && current.ProfileDigest == previous.SourceProfileDigest
}
func (s *Server) verifyAcceptedRetryDelivery(ctx context.Context, expected types.ProviderReplacementSource) error {
	if len(s.procMgr.ListPendingPermissions()) != 0 {
		return journal.ErrProviderReplacementConflict
	}
	delivery, err := s.procMgr.DeliveryRecoveryDescriptor(ctx, expected.StreamID)
	if err != nil {
		return err
	}
	if !delivery.Durable || delivery.Unresolved || delivery.SubmissionsTruncated {
		return journal.ErrProviderReplacementConflict
	}
	return nil
}

func (s *Server) abandonProviderCandidate(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt, version uint64) (journal.ProviderReplacementReceipt, error) {
	if receipt.Phase == journal.ProviderReplacementFailed {
		return receipt, nil
	}
	if receipt.Version != version || (!receipt.SourceTerminated && receipt.PreviousOperationID == "") || receipt.Phase == journal.ProviderReplacementAccepted {
		return receipt, journal.ErrProviderReplacementConflict
	}
	if receipt.TargetScope.BindingID != "" {
		if err := s.mcpServer.RetireBuiltinGeneration(receipt.TargetScope.BindingID); err != nil {
			return receipt, err
		}
	}
	receipt.Phase = journal.ProviderReplacementFailed
	receipt.FailureCode = "target_superseded"
	return storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
}

func (s *Server) providerRetrySourceCanResume(ctx context.Context, storage *journal.Journal, previousID string) (bool, error) {
	if previousID == "" {
		return true, nil
	}
	previous, err := storage.ReadProviderReplacement(ctx, previousID)
	return previous.Phase == journal.ProviderReplacementReleased || (previous.Phase == journal.ProviderReplacementAccepted && s.procMgr.Status() == process.StatusRunning), err
}
