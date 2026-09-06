package github

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Service) handleCIRunPreflightError(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	var requestErr *CIRunRequestError
	if errors.As(err, &requestErr) {
		return s.failCIRunRequest(ctx, request, requestErr.Class)
	}
	classified := classifyCIRunProviderError(err, false, false)
	applyCIRunProviderMetadata(request, GitHubRequestMetadata{}, classified)
	if ciRunFailureFromError(classified) == CIRunFailureProviderRateLimited {
		return s.deferCIRunForRateLimit(ctx, request, classified)
	}
	return s.failCIRunRequest(ctx, request, ciRunFailureFromError(classified))
}

func (s *Service) handleCIRunWorkflowContentReadError(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	classified := classifyCIRunProviderError(err, false, false)
	applyCIRunProviderMetadata(request, GitHubRequestMetadata{}, classified)
	class := ciRunFailureFromError(classified)
	if class == CIRunFailureProviderRateLimited {
		return s.deferCIRunForRateLimit(ctx, request, classified)
	}
	if class == CIRunFailureProviderUnavailable ||
		class == CIRunFailureInstallationPermission || class == CIRunFailureInstallationRequired {
		return s.failCIRunRequest(ctx, request, class)
	}
	return s.failCIRunRequest(ctx, request, CIRunFailureDispatchDenied)
}

func (s *Service) handleCIRunReconciliationReadError(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	classified := classifyCIRunProviderError(err, false, false)
	applyCIRunProviderMetadata(request, GitHubRequestMetadata{}, classified)
	class := ciRunFailureFromError(classified)
	if class != CIRunFailureProviderRateLimited {
		return receiptFromCIRunRequest(request), &CIRunRequestError{Class: class}
	}
	now := s.ciRunClock()().UTC()
	retryAfter := ciRunRetryAfter(now, classified)
	request.ProviderRetryAfter = &retryAfter
	request.FailureClass = string(class)
	request.UpdatedAt = now
	audit := s.newCIRunAuditEvent(request, "provider_rate_limited", class)
	if err := s.store.DeferCIRunReconciliationForRateLimit(ctx, request, retryAfter, now, audit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.reloadCIRunResult(ctx, request)
		}
		return nil, err
	}
	return receiptFromCIRunRequest(request), &CIRunRequestError{Class: class}
}

func (s *Service) deferCIRunForRateLimit(
	ctx context.Context, request *CIRunRequest, err error,
) (*CIRunReceipt, error) {
	now := s.ciRunClock()().UTC()
	retryAfter := ciRunRetryAfter(now, err)
	if err := s.store.DeferCIRunForRateLimitWithAudit(ctx, request, retryAfter, now,
		s.newCIRunAuditEvent(request, "provider_rate_limited", CIRunFailureProviderRateLimited)); err != nil {
		return nil, err
	}
	return receiptFromCIRunRequest(request), &CIRunRequestError{Class: CIRunFailureProviderRateLimited}
}

func ciRunRetryAfter(now time.Time, err error) time.Time {
	retryAfter := now.Add(ciRunDefaultRateLimitDelay)
	var providerErr *CIRunProviderError
	if errors.As(err, &providerErr) && providerErr.RetryAfter != nil && providerErr.RetryAfter.After(now) {
		return providerErr.RetryAfter.UTC()
	}
	return retryAfter
}

func ciRunFailureFromError(err error) CIRunFailureClass {
	var requestErr *CIRunRequestError
	if errors.As(err, &requestErr) {
		return requestErr.Class
	}
	var providerErr *CIRunProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Class
	}
	return CIRunFailureProviderUnavailable
}

func ciRunInstallationFailure(err error) CIRunFailureClass {
	if errors.Is(err, ErrGitHubCapabilityDenied) {
		return CIRunFailureInstallationPermission
	}
	return CIRunFailureInstallationRequired
}
