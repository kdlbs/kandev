package api

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/server/process/probe"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) prepareProviderReplacement(ctx context.Context, storage *journal.Journal, req types.ProviderReplacementRequest) (journal.ProviderReplacementReceipt, error) {
	receipt, err := storage.ReadProviderReplacement(ctx, req.OperationID)
	if err != nil {
		return receipt, err
	}
	if receipt.OperationID != "" {
		if receipt.RequestHash != replacementSourceHash(req.Source) || receipt.PreviousOperationID != req.PreviousOperationID {
			return receipt, journal.ErrProviderReplacementConflict
		}
		return receipt, nil
	}
	expires := time.Now().Add(30 * time.Second).UTC()
	retry := req.PreviousOperationID != ""
	resumeSource, verifyLiveSource, err := s.reserveProviderReplacement(ctx, storage, req, expires)
	if err != nil {
		return receipt, err
	}
	keep := retry
	defer func() {
		if !keep {
			s.procMgr.ReleaseProviderReplacement(req.OperationID)
		}
	}()
	if verifyLiveSource {
		if err := s.verifyProviderReplacementSource(ctx, req.Source); err != nil {
			return receipt, err
		}
	}
	if resumeSource {
		if err := s.mcpServer.FreezeBuiltinGeneration(req.Source.BindingID); err != nil {
			return receipt, err
		}
	}
	keep = true
	receipt = journal.ProviderReplacementReceipt{PreviousOperationID: req.PreviousOperationID, OperationID: req.OperationID, ExecutionID: req.Source.ExecutionID, SessionID: req.Source.SessionID, HelperID: req.Source.HelperID, SourceTaskID: req.Source.TaskID, SourceProfileDigest: req.Source.ProfileDigest, SourceProcessGeneration: req.Source.ProcessGeneration, SourceConfigRevision: req.Source.ConfigurationRevision, SourceStopGeneration: req.Source.StopGeneration, SourceBindingID: req.Source.BindingID, SourceStreamID: req.Source.StreamID, SourceIncarnationID: req.Source.IncarnationID, SourceHarnessGeneration: req.Source.HarnessGeneration, NativeSessionID: req.Source.NativeSessionID, RequestHash: replacementSourceHash(req.Source), ExpiresAt: expires, Phase: journal.ProviderReplacementPrepared}
	receipt, err = storage.CreateProviderReplacement(ctx, receipt)
	if err != nil {
		return receipt, err
	}
	keep = true
	time.AfterFunc(time.Until(expires), func() { s.expirePreparedProvider(req.OperationID) })
	return receipt, nil
}

func (s *Server) verifyProviderReplacementSource(ctx context.Context, expected types.ProviderReplacementSource) error {
	current, err := s.procMgr.ProviderReplacementSource()
	if err != nil {
		return err
	}
	if !s.providerSourceMatches(current, expected) {
		return journal.ErrProviderReplacementConflict
	}
	provider := s.procMgr.GetAdapter()
	replacement, ok := provider.(adapter.ProviderMCPReplacer)
	if !ok {
		return errors.New("provider replacement is unsupported")
	}
	if err := replacement.ProviderMCPReplacementReady(ctx); err != nil {
		return err
	}
	catalog, err := replacement.ProviderMCPCatalog(ctx, expected.NativeSessionID)
	if err != nil {
		return err
	}
	if len(catalog) == 0 {
		return errors.New("provider MCP catalog is unavailable")
	}
	recorder, ok := provider.(adapter.TurnStartRecorder)
	if !ok {
		return errors.New("provider background evidence is unavailable")
	}
	start, ok := recorder.RecordedTurnStart(expected.NativeSessionID)
	if !ok {
		return errors.New("provider turn evidence is unavailable")
	}
	result, err := s.procMgr.ProviderBackgroundWorkloads(start)
	if err != nil {
		return err
	}
	if result != probe.ResultSettled {
		return errors.New("provider background work has not settled")
	}
	descriptor, err := s.procMgr.DeliveryRecoveryDescriptor(ctx, expected.StreamID)
	if err != nil {
		return err
	}
	if !descriptor.Durable || descriptor.Unresolved || descriptor.SubmissionsTruncated {
		return errors.New("provider delivery has not settled")
	}
	return nil
}

func (s *Server) releasePreparedProvider(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt) (journal.ProviderReplacementReceipt, error) {
	canResume, err := s.providerRetrySourceCanResume(ctx, storage, receipt.PreviousOperationID)
	if err != nil {
		return receipt, err
	}
	if receipt.Phase != journal.ProviderReplacementPrepared || !canResume {
		return receipt, journal.ErrProviderReplacementConflict
	}
	source, err := s.procMgr.ProviderReplacementSource()
	if err != nil || source != replacementReceiptSource(receipt) || s.procMgr.Status() != process.StatusRunning {
		return receipt, journal.ErrProviderReplacementConflict
	}
	receipt.Phase = journal.ProviderReplacementReleased
	updated, err := storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	if err != nil {
		return receipt, err
	}
	if err := s.finishReleasedProvider(updated); err != nil {
		return updated, err
	}
	return updated, nil
}

func (s *Server) expirePreparedProvider(id string) {
	s.providerReplacementMu.Lock()
	defer s.providerReplacementMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	storage, err := s.procMgr.DeliveryJournal()
	if err != nil {
		return
	}
	receipt, err := storage.ReadProviderReplacement(ctx, id)
	if err != nil || receipt.Phase != journal.ProviderReplacementPrepared || time.Now().Before(receipt.ExpiresAt) {
		return
	}
	canResume, resumeErr := s.providerRetrySourceCanResume(ctx, storage, receipt.PreviousOperationID)
	if resumeErr != nil {
		return
	}
	if !canResume {
		receipt.Phase = journal.ProviderReplacementFailed
		receipt.FailureCode = "preparation_expired"
		_, _ = storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
		return
	}
	_, _ = s.releasePreparedProvider(ctx, storage, receipt)
}

func (s *Server) providerSourceMatches(current, expected types.ProviderReplacementSource) bool {
	return current == expected && expected.NativeSessionID != "" && expected.ProcessGeneration != 0 && s.procMgr.Status() == process.StatusRunning && s.mcpServer != nil && s.cfg.McpProfile != nil && s.cfg.McpProfile.BuiltinPackages != nil && len(s.procMgr.ListPendingPermissions()) == 0
}

func (s *Server) finishReleasedProvider(receipt journal.ProviderReplacementReceipt) error {
	current, err := s.procMgr.ProviderReplacementSource()
	if err != nil {
		return err
	}
	if current != replacementReceiptSource(receipt) || s.procMgr.Status() != process.StatusRunning {
		return journal.ErrProviderReplacementConflict
	}
	if err := s.mcpServer.ResumeBuiltinGeneration(receipt.SourceBindingID); err != nil {
		return err
	}
	s.procMgr.ReleaseProviderReplacement(receipt.OperationID)
	return nil
}

func (s *Server) reserveProviderReplacement(ctx context.Context, storage *journal.Journal, req types.ProviderReplacementRequest, expires time.Time) (bool, bool, error) {
	if req.PreviousOperationID == "" {
		return true, true, s.procMgr.ReserveProviderReplacement(req.OperationID, expires)
	}
	if err := s.verifyFailedProviderRetry(ctx, storage, req.PreviousOperationID, req.Source); err != nil {
		return false, false, err
	}
	canResume, err := s.providerRetrySourceCanResume(ctx, storage, req.PreviousOperationID)
	if err != nil {
		return false, false, err
	}
	previous, err := storage.ReadProviderReplacement(ctx, req.PreviousOperationID)
	if err != nil {
		return false, false, err
	}
	resumeSource := previous.Phase != journal.ProviderReplacementFailed
	if resumeSource {
		return true, canResume, s.procMgr.ReserveProviderReplacement(req.OperationID, expires)
	}
	return false, canResume, s.procMgr.TakeoverProviderReplacement(req.PreviousOperationID, req.OperationID, expires)
}
