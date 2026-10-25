---
id: 03-legacy-identity
title: Isolate legacy rate identity
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-002.1
  - AC-INTEGRATIONS-GITHUB-RATE-002.4
  - AC-INTEGRATIONS-GITHUB-RATE-002.5
  - AC-INTEGRATIONS-GITHUB-RATE-002.6
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 03: Isolate legacy rate identity

## Summary

Bind legacy clients to one canonical principal without sharing mutable trackers
across different known humans. Preserve same-account limits across rotation.

## Confirmed root cause

`coordinateLegacyClient` passes the same service tracker for every principal.
The coordinator accepts that pointer for each new key. Distinct admissions then
read the same mutable observation state. Startup can also reattach a tracker.

## In scope

Add `TestLegacyAccountReplacementIsolatesRateState` and
the same-account rotation case within that test. Use actual service configuration
with fake identity/provider responses. Cover provisional-to-known binding,
identity probe failure then recovery, old-client in-flight responses, case-normalized
logins, two workspace scopes, and unchanged distinct App-installation isolation.
Trace all RateTracker accessors and startup attachment before choosing ownership
changes. Retain existing workspace authorization and credential-cache boundaries.

## Out of scope

New UI, persistence changes, external publication, and unrelated CI failures.

## Acceptance

- Alice and Bob remain isolated through startup and ConfigureToken paths, for primary and secondary state.
- Same-principal rotation and equivalent managed/legacy identities retain one canonical restriction.
- Unknown identity remains provisional, and later identity resolution cannot transfer another account state.

## Verification

Run from the repository root. First add the named regression and record its
expected failure. Apply the correction, then run:

```bash
(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test.*(Legacy|ConfigureToken|Principal|Coordinator|Admission|CredentialResolver)' -count=1 -timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/provider.go`
- `apps/backend/internal/github/service_tokens.go`
- `apps/backend/internal/github/auth_resolver.go`
- `apps/backend/internal/github/rate_coordinator.go`
- `apps/backend/internal/github/service_legacy_rate_identity_test.go`

## Dependencies

No implementation dependency. Follow the plan order in this single session.

## Risks

Preserve the existing admission, workspace identity, and cancellation contracts.
Use bounded deterministic fixtures and retain provider-call assertions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-rate-limit-coordination.md).
- [System design](../../specs/integrations/system-design/github-rate-limit-coordination.md).
- [Plan evidence and compatible paths](plan.md).
- ADR 0047 and ADR-2026-08-29-github-provider-rate-coordination.

## Results

RED: service replacement, resolver replacement, and coordinator ownership failed.
GREEN: the specified identity/resolver/admission race command passed (15.284s).
The added `TestConfigureTokenRefreshesLegacyResolverCache` failed with Bob still
using Alice after replacement. ConfigureToken and ClearToken now invalidate
resolver caches while retaining the canonical coordinator state.
Existing global-pointer fixtures now assert canonical ownership or client-local probe admission.
