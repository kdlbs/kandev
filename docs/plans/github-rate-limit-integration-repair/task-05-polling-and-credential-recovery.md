---
id: 05-polling-and-credential-recovery
title: Complete polling isolation and credential recovery
status: done
wave: 2
depends_on:
  - 03-legacy-identity
  - 04-watch-progress
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-002.2
  - AC-INTEGRATIONS-GITHUB-RATE-002.6
  - AC-INTEGRATIONS-GITHUB-RATE-002.7
  - AC-INTEGRATIONS-GITHUB-RATE-003.7
  - AC-INTEGRATIONS-GITHUB-RATE-003.8
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 05: Complete polling isolation and credential recovery

## Summary

Remediate the independent review of head `2f2e26583c3717d4aedebfc17e0fa99d368dcabf`.
Retain the existing principal coordinator and Workflow Sync continuation owner.

## Confirmed root cause

Fixed workspace order lets the first PR-monitoring workspace consume each new
admission opportunity. Local admission deferrals incorrectly open transient
authentication circuits, which also defeats rotation. Global legacy quota checks in PR monitoring and review
cleanup can block a different healthy principal. The recovery fingerprint omits
legacy secret revisions and applied App registration key generations.

## In scope

Rotate PR-monitoring workspace priority. Remove global quota gates while keeping
resolved-principal admission and background reserve checks. Carry existing secret
update metadata through the GitHub adapter and include it in legacy fingerprints.
Serialize legacy fingerprint reads with managed secret mutation and client/cache
publication. Invalidate stale resolver entries, auth-dependent caches, and retained polling
progress before publishing a revision
observed after a generic Secrets API edit. Include App registration generation and availability in App fingerprints.
Cover restart stability, token removal, and both principal-isolation paths.

## Out of scope

New schedulers, schema migrations, UI changes, and arbitrary external CLI account
changes that do not update managed connection or secret metadata.

## Acceptance

- Each eligible PR workspace receives reads under shared-principal pacing.
- Legacy exhaustion cannot block an independent principal's monitoring or cleanup.
- Background requests retain their own principal's quota reserve.
- Managed credential changes invalidate retry and continuation identity.
- Unchanged credential metadata survives restart without token disclosure.

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_circuit.go`
- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/service_tokens.go`
- `apps/backend/internal/github/service_legacy_publication_test.go`
- `apps/backend/internal/github/service_legacy_revision_test.go`
- `apps/backend/internal/github/service_legacy_permission_test.go`
- `apps/backend/internal/github/service_review_cleanup_scheduled_test.go`
- `apps/backend/internal/github/poller_review_cleanup.go`
- `apps/backend/internal/github/service_connections.go`
- `apps/backend/internal/github/factory.go`
- `apps/backend/internal/github/poller_credential_recovery_test.go`
- `apps/backend/internal/github/poller_test.go`
- `apps/backend/internal/github/service_token_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/services_github_fingerprint_test.go`

## Verification

Run from `apps/backend`:

```bash
go test -trimpath -race ./internal/github ./internal/workflowsync -count=1 -timeout=10m
go test -trimpath -race ./internal/backendapp -run '^TestGitHubLegacyFingerprintFollowsDurableSecretRevision$' -count=1 -timeout=5m
golangci-lint run ./... --new-from-rev=371d9c18193af5893fbe70387fee9330f7dbb65b --timeout=5m
```

Run specification and public-documentation checks from the repository root.
Validate the final changes on the current-base merge tree. Refresh exact-head CI
after delivery. The independent reviewer rechecks the remediation.

## Results

The new regressions failed before their corresponding production changes.
The initial current-base synthetic merge passed GitHub and Workflow Sync race
suites (298.341s and 1.402s). That result predates Task 05 and is historical.
Final verification on 2026-10-22:

- Final focused race tests passed: GitHub 44.198s and backend application 36.608s.
  The selection covered legacy credentials, fingerprints, tokens, PR watches,
  cleanup, polling, repository caches, and batched queries.
- Full GitHub and Workflow Sync race suites passed on the final patch combined
  with current main `5aa31bec1a8a5e9cdb679811b02aa3e7b78271c2` (190.297s and 2.070s).
- Full backend lint against recorded PR base
  `371d9c18193af5893fbe70387fee9330f7dbb65b`: zero issues.
- Specification catalog and lint, public docs, work-order coverage, and
  whitespace checks passed.
- The independent reviewer reproduced the permission-cache regression after
  remediation: the replacement credential is called and its denial is returned.
  No remaining blocker was found in the scoped remediation.

The final focused command was:

```bash
go test -trimpath -race ./internal/github ./internal/backendapp -run 'Test.*(Legacy|Fingerprint|Token|PRWatch|PRMonitoring|ReviewCleanup|WatchPolling|Repo.*Cache|Cache.*Repo|Batched)' -count=1 -timeout=8m
```

Delivery and exact-head CI are recorded in the session. Live GitHub throttling
and local browser E2E were not exercised. External CLI account changes without
managed metadata remain outside this repair.
