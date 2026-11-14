package usage

import (
	"context"
	"encoding/json"
)

func (s *UsageService) executionClient(profileID string, source executionUsageSource) executionAccountClient {
	key := "execution_usage:" + profileID
	s.mu.Lock()
	defer s.mu.Unlock()
	if registration, exists := s.clients[key]; exists {
		if client, ok := registration.client.(executionAccountClient); ok && client.matchesSource(source) {
			return client
		}
	}
	client := newExecutionAccountClient(source)
	s.clients[key] = registration{client: client}
	return client
}

// GetExecutionUsage joins a runtime-owned source with actual provider account
// references. Profile IDs and cache addresses never establish account identity.
func (s *UsageService) GetExecutionUsage(ctx context.Context, input ExecutionUsageInput) (*ExecutionUsage, error) {
	source, reason := resolveExecutionUsageSource(input)
	if reason != "" {
		return &ExecutionUsage{Reason: reason}, nil
	}
	client := s.executionClient(input.ProfileID, source)
	identity, err := client.accountIdentity(ctx)
	if err != nil {
		return &ExecutionUsage{Reason: "account_unavailable"}, err
	}
	if identity.account == "" {
		return &ExecutionUsage{Reason: "account_unknown"}, nil
	}
	if input.ExpectedAccountRef != "" && (identity.account != input.ExpectedAccountRef || identity.organization != input.ExpectedOrganizationRef) {
		return &ExecutionUsage{Reason: executionBindingChanged}, nil
	}
	// Existing provider IDs form the observation address; it is not a new
	// credential fingerprint, account ID, quota counter, or authority record.
	address, _ := json.Marshal([]string{source.provider, identity.organization, identity.account})
	key := "execution_usage:" + string(address)
	current, _ := s.cache.SnapshotPair(key)
	if current != nil && !executionAccountMatches(current, source.provider, identity) {
		s.cache.Invalidate(key)
	}
	current, err = s.cache.GetOrFetch(ctx, key, client.FetchUsage)
	if err != nil {
		return &ExecutionUsage{Reason: "observation_unavailable"}, err
	}
	if !executionAccountMatches(current, source.provider, identity) {
		return &ExecutionUsage{Reason: executionBindingChanged}, nil
	}
	current, previous := s.cache.SnapshotPair(key)
	if !executionAccountMatches(current, source.provider, identity) {
		return &ExecutionUsage{Reason: "observation_unavailable"}, nil
	}
	return &ExecutionUsage{SourceKnown: true, Reason: "ok", Current: current, Previous: previous}, nil
}

func executionAccountMatches(sample *ProviderUsage, provider string, identity executionAccount) bool {
	return sample != nil && sample.Provider == provider && sample.AccountRef == identity.account && sample.OrganizationRef == identity.organization
}
