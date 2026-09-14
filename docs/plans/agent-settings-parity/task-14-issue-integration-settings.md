---
id: "14-issue-integration-settings"
title: "Expose issue integration settings"
status: done
wave: 14
depends_on:
  - "13-notification-settings"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-003
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-004
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-006
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-007
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-008
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-009
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-006.8
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-004.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-008.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-008.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-008.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-008.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-008.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-008.6
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.3
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
  - ../../specs/platform/system-design/agent-settings-domains.md
---

# Task 14: Expose issue integration settings

## Summary

Deliver the domain through the shared settings tools and authorized resource lookup.
Include its complete eligible field set, current UI result, and independent contract checks.

## In scope

- Adopt noncredential Jira and Linear defaults plus existing Sentry instance settings and all eligible watch fields.
- Preserve workspace ownership and Sentry instance identity. Do not select a singleton when several instances exist.
- Reuse credential-preserving domain paths. Enrollment and credential-bearing connection identity changes remain interactive-only.
- Validate watch workflow/profile/instance references, schedules, and feature availability before saving.
- Retain existing cache invalidation and background health behavior. Report post-save probe failure as degraded status, not rollback.
- Reconcile integration forms and watch editors using scoped events or invalidation.
- Register metadata, context schemas, typed updates, redacted reads, and authorized lookup in the production composition.
- Add domain aliases to the independent search-query suite and update the generated snapshots.
- Include shared DTO conversion, semantic validation, and required atomic patch behavior for every write interface.
- Use current domain events, or add scoped value-free invalidation when none exists.
- Add coverage bindings and scoped editor tests. Update `docs/public/automation-and-mcp.md` with delivered behavior.

## Out of scope

- New persistence owners, arbitrary database access, privilege changes, and unrelated lifecycle operations.
- Automatic merging of dirty drafts, new navigation layouts, and real provider credentials in tests.

## Acceptance

- Registered MCP calls match the existing UI contract for every eligible field, including explicit empties and domain-specific errors.
- Unauthorized, invalid, and conflicting changes preserve invariants without partial writes or sensitive output. New atomic patches cover competing writers.
- Clean editors update live and survive reload. Dirty drafts survive until recovery, on desktop and phone.

## Verification

Run from the repository root after the one-time workspace dependency install in Task 00.
These new test paths and names are planned implementation outputs, not existing evidence.

```bash
rtk proxy go -C apps/backend test ./internal/jira ./internal/linear ./internal/sentry ./internal/mcp/server ./internal/mcp/handlers
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk pnpm --dir apps/web exec vitest run lib/settings-discovery/issue-integration-settings-contract.test.ts components/settings/issue-integration-settings-reconciliation.test.tsx
rtk pnpm --dir apps/web run typecheck
rtk pnpm --dir apps/web run i18n:check
rtk pnpm --dir apps/web e2e:run --project chromium tests/settings/issue-integration-settings-mcp.spec.ts
rtk pnpm --dir apps/web e2e:run --project mobile-chrome tests/settings/mobile-issue-integration-settings-mcp.spec.ts
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

Add `TestIssueIntegrationSettingsParity` and `TestIssueIntegrationSettingsAuthority` in
`apps/backend/internal/mcp/server/settings_issue_integration_settings_test.go`.
Use real registered tools plus domain fixtures, not only direct handler invocation.
Add domain tests for any changed persistence path, including rollback and concurrent updates.
When a SQL query changes, include SQLite/PostgreSQL behavior evidence under the existing domain test conventions.

## E2E scenario

Update one fixture per provider, observe each settings/watch surface, and reject a foreign workspace or Sentry instance.
Use disposable resources and mock providers. Restore user-level settings in cleanup, including after failures.
Run desktop and phone separately with the managed runner and record discovered test counts.

## Files likely touched

- `apps/backend/internal/jira/models.go`
- `apps/backend/internal/jira/service.go`
- `apps/backend/internal/linear/models.go`
- `apps/backend/internal/linear/service.go`
- `apps/backend/internal/sentry/models.go`
- `apps/backend/internal/sentry/service.go`
- `apps/backend/internal/{jira,linear,sentry}/settings_catalog.go (new)`
- `apps/web/app/settings/integrations/`
- `apps/backend/internal/backendapp/services.go` and MCP handler composition.
- `apps/backend/internal/mcp/server/settings_issue_integration_settings_test.go` (new).
- `apps/web/lib/settings-discovery/issue-integration-settings-contract.test.ts` (new).
- `apps/web/components/settings/issue-integration-settings-reconciliation.test.tsx` (new): clean updates, dirty conflicts, own acknowledgements, and late replies.
- `apps/web/lib/settings-discovery/coverage-inventory.ts` and generated snapshots.
- Relevant domain store subscriptions, current editor state, and locale catalogs.
- The two E2E files named in Verification.

## Dependencies

Task 13 completes the preceding slice.
Tasks 00 through 04 establish inventory, schemas, generic tools, and draft-recovery behavior.
The sequential order avoids conflicts in shared composition and generated snapshots.

## Risks

- Legacy full-document setters can erase omitted values. Shared domain atomic patches or complete-document semantics must protect every writer.
- Saved settings and active runtime state can differ. Responses must not claim an unobserved runtime change.
- Missing identity can look like an internal service caller. MCP must reject that case under enforced authentication.

## Mobile contract

Reuse the current domain settings route, shared save contributor, and existing phone composition.
Keep recovery inline with the existing discard action, one content scroll owner, and safe-area-aware controls.
Prove touch access and no horizontal page overflow. No new modal or breakpoint layout is required.

## Parallelism

`sequential`

## Inputs

- Both system designs, especially Required domains, Mutation semantics, Relationships, and Visible results.
- Task 00's field inventory and each source contract listed above.
- Existing domain tests, profile parity tests, and nearby shipped settings E2E.
- Scoped AGENTS.md, /tdd, /mobile-parity, /e2e, and /docs-maintainer before implementation.

## Results

Completed on 2026-09-09.

- Registered Jira, Linear, Sentry instance, and issue-watch settings.
- Routed noncredential fields through provider-owned integration services with workspace authorization.
- Preserved connection and secret-enrollment exceptions and covered provider adapters in backend tests.
