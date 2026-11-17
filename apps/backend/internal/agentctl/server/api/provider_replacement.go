package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (s *Server) handleProviderReplacement(c *gin.Context) {
	if !c.GetBool(authenticatedControlContextKey) {
		c.JSON(http.StatusForbidden, gin.H{"error": "provider replacement requires authenticated control"})
		return
	}
	if c.Request.Method == http.MethodGet {
		s.handleProviderReplacementStatus(c)
		return
	}
	var req types.ProviderReplacementRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF || req.OperationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider replacement request"})
		return
	}
	s.providerReplacementMu.Lock()
	defer s.providerReplacementMu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 60*time.Second)
	defer cancel()
	owned, release, err := s.procMgr.BeginOwnedOperation(ctx)
	if err == nil {
		defer release()
	}
	var receipt journal.ProviderReplacementReceipt
	if err == nil {
		receipt, err = s.providerReplacementAction(owned, req)
	}
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "provider replacement cannot advance", "reason": "provider_replacement_conflict"})
		return
	}
	c.JSON(http.StatusOK, replacementReceiptStatus(receipt))
}

func (s *Server) handleProviderReplacementStatus(c *gin.Context) {
	storage, err := s.procMgr.DeliveryJournal()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "provider replacement storage is unavailable"})
		return
	}
	receipt, err := storage.ReadProviderReplacement(c.Request.Context(), c.Query("operation_id"))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "provider replacement status is unavailable"})
		return
	}
	if receipt.OperationID != "" {
		c.JSON(http.StatusOK, replacementReceiptStatus(receipt))
		return
	}
	source, err := s.procMgr.ProviderReplacementSource()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "provider replacement source is unavailable"})
		return
	}
	status := types.ProviderReplacementStatus{Phase: "idle", Source: source}
	if id := c.Query("operation_id"); id != "" {
		status.Phase = "missing"
		status.OperationID = id
	}
	if support, ok := s.procMgr.GetAdapter().(interface{ ProviderMCPReplacementSupported() bool }); ok && support.ProviderMCPReplacementSupported() {
		status.Capability = types.ProviderMCPReplacementCapability
	}
	if err := s.verifyProviderReplacementSource(c.Request.Context(), source); err == nil {
		status.Ready = true
		status.Capability = types.ProviderMCPReplacementCapability
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) providerReplacementAction(ctx context.Context, req types.ProviderReplacementRequest) (journal.ProviderReplacementReceipt, error) {
	storage, err := s.procMgr.DeliveryJournal()
	if err != nil {
		return journal.ProviderReplacementReceipt{}, err
	}
	if req.Action == types.ProviderReplacementActionPrepare {
		return s.prepareProviderReplacement(ctx, storage, req)
	}
	receipt, err := storage.ReadProviderReplacement(ctx, req.OperationID)
	if err != nil {
		return receipt, err
	}
	if receipt.OperationID == "" || replacementReceiptSource(receipt) != req.Source {
		return receipt, journal.ErrProviderReplacementConflict
	}
	if req.Action == types.ProviderReplacementActionAbandon {
		return s.abandonProviderCandidate(ctx, storage, receipt, req.ExpectedVersion)
	}
	if req.Action == types.ProviderReplacementActionActivate {
		return s.activateProviderReplacement(ctx, storage, receipt, req)
	}
	if req.Action == types.ProviderReplacementActionAccept {
		return s.acceptProviderReplacement(ctx, storage, receipt, req)
	}
	if req.Action == types.ProviderReplacementActionRetire {
		return s.retirePreparedProvider(ctx, storage, receipt, req.ExpectedVersion)
	}
	if req.Action == types.ProviderReplacementActionRelease {
		if receipt.Phase == journal.ProviderReplacementReleased {
			return receipt, s.finishReleasedProvider(receipt)
		}
		if receipt.Version != req.ExpectedVersion {
			return receipt, journal.ErrProviderReplacementConflict
		}
		return s.releasePreparedProvider(ctx, storage, receipt)
	}
	return receipt, errors.New("unsupported provider replacement action")
}

func replacementSourceHash(source types.ProviderReplacementSource) string {
	raw, _ := json.Marshal(source)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func replacementReceiptSource(r journal.ProviderReplacementReceipt) types.ProviderReplacementSource {
	return types.ProviderReplacementSource{ExecutionID: r.ExecutionID, TaskID: r.SourceTaskID, SessionID: r.SessionID, HelperID: r.HelperID, ProcessGeneration: r.SourceProcessGeneration, ConfigurationRevision: r.SourceConfigRevision, StopGeneration: r.SourceStopGeneration, BindingID: r.SourceBindingID, ProfileDigest: r.SourceProfileDigest, HarnessGeneration: r.SourceHarnessGeneration, StreamID: r.SourceStreamID, IncarnationID: r.SourceIncarnationID, NativeSessionID: r.NativeSessionID}
}

func replacementReceiptStatus(r journal.ProviderReplacementReceipt) types.ProviderReplacementStatus {
	status := types.ProviderReplacementStatus{OperationID: r.OperationID, Phase: string(r.Phase), Version: r.Version, Source: replacementReceiptSource(r), FailureCode: r.FailureCode, SourceTerminated: r.SourceTerminated, NativeResumeConfirmed: r.NativeResumeConfirmed, Catalog: r.Catalog}
	if r.TargetScope.BindingID != "" {
		scope := r.TargetScope
		status.Scope = &scope
	}
	return status
}
