package api

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func (s *Server) retirePreparedProvider(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt, version uint64) (journal.ProviderReplacementReceipt, error) {
	switch receipt.Phase {
	case journal.ProviderReplacementRetired, journal.ProviderReplacementStarting, journal.ProviderReplacementStarted, journal.ProviderReplacementAccepted:
		return receipt, nil
	case journal.ProviderReplacementPrepared:
		if receipt.Version != version || !time.Now().Before(receipt.ExpiresAt) {
			return receipt, journal.ErrProviderReplacementConflict
		}
		if err := s.verifyPreparedProvider(ctx, storage, receipt); err != nil {
			return receipt, err
		}
		receipt.Phase = journal.ProviderReplacementRetiring
		var err error
		receipt, err = storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
		if err != nil {
			return receipt, err
		}
	case journal.ProviderReplacementRetiring:
	default:
		return receipt, journal.ErrProviderReplacementConflict
	}
	if err := s.mcpServer.RetireBuiltinGeneration(receipt.SourceBindingID); err != nil {
		return receipt, err
	}
	if s.procMgr.Status() == process.StatusRunning || (receipt.PreviousOperationID != "" && s.procMgr.Status() == process.StatusError) {
		stop := s.procMgr.RetireProviderReplacement
		if receipt.PreviousOperationID != "" {
			stop = s.procMgr.RetireProviderReplacementCandidate
		}
		if err := stop(ctx, replacementReceiptSource(receipt)); err != nil {
			return receipt, err
		}
	}
	return s.recordProviderTermination(ctx, storage, receipt)
}

func (s *Server) recordProviderTermination(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt) (journal.ProviderReplacementReceipt, error) {
	current, err := s.procMgr.ProviderReplacementSource()
	if err != nil {
		return receipt, err
	}
	current.NativeSessionID = receipt.NativeSessionID
	if current != replacementReceiptSource(receipt) || !s.procMgr.ProviderTerminationConfirmed() {
		return receipt, journal.ErrProviderReplacementConflict
	}
	receipt.Phase = journal.ProviderReplacementRetired
	receipt.SourceTerminated = true
	return storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
}

func (s *Server) verifyPreparedProvider(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt) error {
	if receipt.PreviousOperationID != "" {
		return s.verifyFailedProviderRetry(ctx, storage, receipt.PreviousOperationID, replacementReceiptSource(receipt))
	}
	return s.verifyProviderReplacementSource(ctx, replacementReceiptSource(receipt))
}
