package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func providerActivationHash(req types.ProviderReplacementRequest) string {
	req.Action = ""
	req.ExpectedVersion = 0
	req.ScopeCredential = ""
	raw, _ := json.Marshal(req)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (s *Server) activateProviderReplacement(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt, req types.ProviderReplacementRequest) (journal.ProviderReplacementReceipt, error) {
	if receipt.Phase == journal.ProviderReplacementRetired {
		var err error
		receipt, err = s.startProviderActivation(ctx, storage, receipt, req)
		if err != nil {
			return receipt, err
		}
	} else if receipt.ActivationHash != providerActivationHash(req) {
		return receipt, journal.ErrProviderReplacementConflict
	}
	if receipt.Phase == journal.ProviderReplacementStarted || receipt.Phase == journal.ProviderReplacementAccepted || receipt.Phase == journal.ProviderReplacementFailed {
		return receipt, nil
	}
	if receipt.Phase != journal.ProviderReplacementStarting {
		return receipt, journal.ErrProviderReplacementConflict
	}
	catalog, err := s.verifyProviderActivation(ctx, receipt)
	if err != nil {
		return s.failProviderActivation(ctx, storage, receipt, "provider_resume_or_catalog_failed")
	}
	receipt.Catalog = catalog
	receipt.NativeResumeConfirmed = true
	receipt.Phase = journal.ProviderReplacementStarted
	return storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
}

func (s *Server) prepareProviderActivation(req types.ProviderReplacementRequest) error {
	if req.Scope.ExecutionID != req.Source.ExecutionID || req.Scope.HelperID != req.Source.HelperID || req.Scope.ProcessGeneration != req.Source.ProcessGeneration+1 || req.Scope.HarnessGeneration != req.Source.HarnessGeneration || req.TargetStreamID == "" || req.TargetStreamID != req.Source.StreamID {
		return journal.ErrProviderReplacementConflict
	}
	return s.mcpServer.ValidateBuiltinGeneration(req.Scope, req.ScopeCredential, req.Profile)
}

func (s *Server) verifyProviderActivation(ctx context.Context, receipt journal.ProviderReplacementReceipt) (map[string][]string, error) {
	if !s.providerActivationMatches(receipt) {
		return nil, journal.ErrProviderReplacementConflict
	}
	provider := s.procMgr.GetAdapter()
	replacement, ok := provider.(adapter.ProviderMCPReplacer)
	if !ok {
		return nil, errors.New("provider replacement verification is unsupported")
	}
	if provider.GetAgentInfo() == nil {
		if err := provider.Initialize(ctx); err != nil {
			return nil, err
		}
	}
	if support, ok := provider.(interface{ ProviderMCPReplacementSupported() bool }); ok && !support.ProviderMCPReplacementSupported() {
		return nil, errors.New("provider replacement version is unverified")
	}
	if provider.GetSessionID() == "" {
		var servers []types.McpServer
		for _, server := range s.cfg.McpServers {
			servers = append(servers, types.McpServer{Name: server.Name, Type: server.Type, URL: server.URL, Command: server.Command, Args: slices.Clone(server.Args), Env: maps.Clone(server.Env), Headers: maps.Clone(server.Headers), BuiltinPackage: p.BuiltinPackageForRuntimeName(server.Name)})
		}
		if err := provider.LoadSession(ctx, receipt.NativeSessionID, servers); err != nil {
			return nil, err
		}
	}
	if provider.GetSessionID() != receipt.NativeSessionID {
		return nil, errors.New("provider resumed a different native conversation")
	}
	catalog, err := replacement.ProviderMCPCatalog(ctx, receipt.NativeSessionID)
	if err != nil {
		return nil, err
	}
	expected, err := s.mcpServer.BuiltinGenerationCatalog(receipt.TargetScope.BindingID)
	if err != nil {
		return nil, err
	}
	if err := verifyProviderBuiltinCatalog(expected, catalog); err != nil {
		return nil, err
	}
	if !s.providerActivationMatches(receipt) {
		return nil, journal.ErrProviderReplacementConflict
	}
	return catalog, nil
}

func verifyProviderBuiltinCatalog(expected, actual map[string][]string) error {
	for server, names := range expected {
		raw, found := actual[server]
		if !found {
			return errors.New("provider omitted a selected built-in MCP package")
		}
		observed := slices.Clone(raw)
		slices.Sort(observed)
		if !slices.Equal(names, observed) {
			return errors.New("provider MCP catalog does not match the saved binding")
		}
	}
	for server := range actual {
		if p.ReservedBuiltinRuntimeName(server) {
			if _, selected := expected[server]; !selected {
				return errors.New("provider retained an excluded built-in MCP package")
			}
		}
	}
	return nil
}

func (s *Server) providerActivationMatches(receipt journal.ProviderReplacementReceipt) bool {
	current, err := s.procMgr.ProviderReplacementSource()
	return err == nil && s.procMgr.Status() == process.StatusRunning && current.ExecutionID == receipt.ExecutionID && current.HelperID == receipt.HelperID && current.ProcessGeneration == receipt.TargetScope.ProcessGeneration && current.StopGeneration == receipt.SourceStopGeneration && current.BindingID == receipt.TargetScope.BindingID && current.ProfileDigest == receipt.TargetScope.ProfileDigest && current.StreamID == receipt.TargetStreamID && current.HarnessGeneration == receipt.TargetScope.HarnessGeneration
}

func (s *Server) failProviderActivation(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt, reason string) (journal.ProviderReplacementReceipt, error) {
	receipt.Phase = journal.ProviderReplacementFailed
	receipt.FailureCode = reason
	return storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
}

func (s *Server) acceptProviderReplacement(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt, req types.ProviderReplacementRequest) (journal.ProviderReplacementReceipt, error) {
	if !receipt.NativeResumeConfirmed || receipt.TargetScope != req.Scope || !s.providerActivationMatches(receipt) || s.procMgr.GetSessionID() != receipt.NativeSessionID {
		return receipt, journal.ErrProviderReplacementConflict
	}
	if receipt.Phase == journal.ProviderReplacementStarted {
		if receipt.Version != req.ExpectedVersion {
			return receipt, journal.ErrProviderReplacementConflict
		}
		receipt.Phase = journal.ProviderReplacementAccepted
		var err error
		receipt, err = storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
		if err != nil {
			return receipt, err
		}
	} else if receipt.Phase != journal.ProviderReplacementAccepted {
		return receipt, journal.ErrProviderReplacementConflict
	}
	if err := s.mcpServer.AcceptBuiltinGeneration(receipt.TargetScope.BindingID); err != nil {
		return receipt, err
	}
	s.procMgr.SetBuiltinToolCatalog(func() map[p.PackageID][]string {
		return s.mcpServer.BuiltinGenerationToolCatalog(receipt.TargetScope.BindingID)
	})
	s.procMgr.ReleaseProviderReplacement(receipt.OperationID)
	return receipt, nil
}

func (s *Server) startProviderActivation(ctx context.Context, storage *journal.Journal, receipt journal.ProviderReplacementReceipt, req types.ProviderReplacementRequest) (journal.ProviderReplacementReceipt, error) {
	if receipt.Version != req.ExpectedVersion || !receipt.SourceTerminated {
		return receipt, journal.ErrProviderReplacementConflict
	}
	if req.ScopeCredential == "" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return receipt, err
		}
		req.ScopeCredential = hex.EncodeToString(secret)
	}
	if err := s.prepareProviderActivation(req); err != nil {
		return receipt, err
	}
	encoded, err := p.MarshalBuiltinBinding(req.Profile)
	if err != nil {
		return receipt, err
	}
	receipt.TargetProfile = encoded
	receipt.TargetScope = req.Scope
	receipt.TargetStreamID = req.TargetStreamID
	receipt.TargetCredential = req.ScopeCredential
	receipt.ActivationHash = providerActivationHash(req)
	receipt.Phase = journal.ProviderReplacementStarting
	receipt, err = storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	if err != nil {
		return receipt, err
	}
	if err := s.mcpServer.PrepareBuiltinGeneration(req.Scope, receipt.TargetCredential, req.Profile); err != nil {
		return s.failProviderActivation(ctx, storage, receipt, "provider_scope_prepare_failed")
	}
	if _, err := s.procMgr.ActivateProviderReplacement(ctx, req); err != nil {
		return s.failProviderActivation(ctx, storage, receipt, "provider_start_failed")
	}

	return receipt, nil
}
