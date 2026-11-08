package usage

import (
	"context"
	"encoding/json"
	"time"
)

// PeekPreparedExecutionAccount reads only an existing native source's identity.
// It never refreshes credentials, requests a provider profile, or fetches usage.
func (s *UsageService) PeekPreparedExecutionAccount(ctx context.Context, input ExecutionUsageInput) (provider, account, organization string, known bool) {
	if s == nil {
		return "", "", "", false
	}
	source, reason := resolveExecutionUsageSource(input)
	if reason != "" {
		return "", "", "", false
	}
	identity, ok := peekExecutionAccount(ctx, s.peekExecutionClient(source))
	if !ok {
		return "", "", "", false
	}
	return source.provider, identity.account, identity.organization, true
}

// PeekExecutionUsage reads already measured facts for this native pin. It never
// refreshes OAuth credentials, requests a provider profile, or fetches usage.
func (s *UsageService) PeekExecutionUsage(ctx context.Context, input ExecutionUsageInput) *ExecutionUsageReport {
	if input.ExpectedAccountRef == "" {
		return &ExecutionUsageReport{Reason: "account_unknown", Model: input.Model}
	}
	return s.peekExecutionUsage(ctx, input)
}

// The input belongs to a native prepared request, composed with that native
// executor's inherited environment. A cold cache cannot trigger a provider probe.
func (s *UsageService) PeekPreparedExecutionUsage(ctx context.Context, input ExecutionUsageInput) *ExecutionUsageReport {
	if reason := NativeProviderSelectionReason(input); reason != "" {
		return &ExecutionUsageReport{Reason: reason, Model: input.Model}
	}
	return s.peekExecutionUsage(ctx, input)
}

func (s *UsageService) peekExecutionUsage(ctx context.Context, input ExecutionUsageInput) *ExecutionUsageReport {
	report := &ExecutionUsageReport{Reason: "observation_unavailable", Model: input.Model}
	source, reason := resolveExecutionUsageSource(input)
	if reason != "" {
		report.Reason = reason
		return report
	}
	client := s.peekExecutionClient(source)
	if client == nil {
		return report
	}
	identity, known := peekExecutionAccount(ctx, client)
	if !known {
		return report
	}
	if input.ExpectedAccountRef != "" && (identity.account != input.ExpectedAccountRef || identity.organization != input.ExpectedOrganizationRef) {
		report.Reason = executionBindingChanged
		report.BindingChanged = true
		return report
	}
	address, _ := json.Marshal([]string{source.provider, identity.organization, identity.account})
	current, previous := s.cache.SnapshotPair("execution_usage:" + string(address))
	if !executionAccountMatches(current, source.provider, identity) {
		return report
	}
	report.SourceKnown = true
	report.Reason = "ok"
	report.Current, report.Previous = observedUsageSample(current), observedUsageSample(previous)
	if limit := executionUsageLimit(current, input.Model, time.Now()); limit != nil {
		report.Limited = true
		report.BlockingWindow = limit.Window
		report.ResetAt = limit.ResetAt
	}
	return report
}

func (s *UsageService) peekExecutionClient(source executionUsageSource) executionAccountClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, registered := range s.clients {
		if client, ok := registered.client.(executionAccountClient); ok && client.matchesSource(source) {
			return client
		}
	}
	// Codex identity is available offline. Claude requires an existing profile
	// identity proved with the exact token; do not create/refresh one here.
	if source.provider == executionProviderOpenAI {
		return newExecutionAccountClient(source)
	}
	return nil
}

func peekExecutionAccount(ctx context.Context, client executionAccountClient) (executionAccount, bool) {
	switch actual := client.(type) {
	case *codexExecutionUsageClient:
		identity, err := actual.accountIdentity(ctx) // local auth file only, no refresh
		return identity, err == nil && identity.account != ""
	case *claudeExecutionUsageClient:
		token := actual.directToken
		if token == "" {
			credentials, err := actual.base.readCredentials()
			if err != nil || credentials.ClaudeAiOauth == nil {
				return executionAccount{}, false
			}
			token = credentials.ClaudeAiOauth.AccessToken
		}
		actual.mu.Lock()
		defer actual.mu.Unlock()
		known := token != "" && token == actual.identityToken && !actual.identityAt.IsZero() && time.Since(actual.identityAt) >= 0 && time.Since(actual.identityAt) <= cacheTTL
		return actual.identity, known && actual.identity.account != ""
	default:
		return executionAccount{}, false
	}
}
