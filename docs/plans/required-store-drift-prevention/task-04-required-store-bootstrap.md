---
id: "04-required-store-bootstrap"
title: "Enforce required-store bootstrap"
status: done
wave: 4
depends_on:
  - "01-required-store-catalog"
  - "02-shared-sql-rendering"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.3
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 04: Enforce required-store bootstrap

## Summary

Wire every catalog store through the required startup tracker. Separate local store creation from remote provider availability.

## In scope

- Pass one tracker through all bootstrap phases.
- Initialize feature-gated and provider-owned SQL schemas before service activation.
- Make schema metadata, telemetry, share, delivery, plugin, and provider store errors fatal.
- Pass initialized stores into provider and feature services.
- Keep credentials, provider probes, migrations from remote accounts, and poller starts degradable.
- Add error-injection and provider-isolation tests.

## Out of scope

- Add periodic runtime probes.
- Change provider authentication behavior or retry policy.
- Change feature enablement.

## Acceptance

- Each catalog ID has exactly one successful bootstrap visit before readiness.
- Any injected store constructor error returns a startup error with the store ID.
- Each injected external provider error leaves all stores and unrelated provider services available.

## Verification

```bash
go test -race ./internal/backendapp -run '^Test(RequiredStoreBootstrap|RequiredStoreFailure|ExternalProviderFailureIsolation|PostgresBootInitializesRepositories)$' -v
```

## Files likely touched

- `apps/backend/internal/backendapp/storage.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/worktree.go`
- `apps/backend/internal/backendapp/postgres_boot_test.go`
- `apps/backend/internal/backendapp/required_store_bootstrap_test.go`
- `apps/backend/internal/github/provider.go`
- `apps/backend/internal/gitlab/provider.go`
- `apps/backend/internal/jira/provider.go`
- `apps/backend/internal/linear/provider.go`
- `apps/backend/internal/sentry/provider.go`
- `apps/backend/internal/azuredevops/provider.go`
- `apps/backend/internal/workflowsync/provider.go`
- `apps/backend/internal/office/configsync/provider.go`

## Dependencies

- Task 01 defines the catalog and tracker.
- Task 02 provides the shared schema boundary for extracted stores.

## Risks

- Provider cleanup functions can be lost when constructors split into two phases.

## Parallelism

`sequential`

## Inputs

- System design sections: Bootstrap contract, External provider isolation.
- Existing provider pattern in `apps/backend/AGENTS.md`.

## Results

Implemented tracker wiring across repository, service, orchestrator, storage,
delivery, plugin, and provider phases. Required local schema failures are fatal
and remote credential or probe failures remain degradable. The focused
bootstrap, fatal-error, provider-isolation, and PostgreSQL boot tests passed.
