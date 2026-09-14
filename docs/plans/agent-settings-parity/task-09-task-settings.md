---
id: "09-task-settings"
title: "Expose task configuration settings"
status: done
wave: 9
depends_on:
  - "08-executor-settings"
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
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-006.5
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

# Task 09: Expose task configuration settings

## Summary

Deliver the domain through the shared settings tools and authorized resource lookup.
Include its complete eligible field set, current UI result, and independent contract checks.

## In scope

- Adopt editable task metadata and UI-supported launch defaults through current task services.
- Classify state, workflow movement, session start/stop, runtime model changes, and arbitrary metadata as non-settings operations.
- Define schemas for known editable metadata fields. Do not expose arbitrary metadata writes through an open object.
- Retain active-session and deferred-launch restrictions. Authorize task/resource pairs before dependencies or writes.
- Publish through existing task events and verify both task details and list projections.
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
rtk proxy go -C apps/backend test ./internal/task/service ./internal/task/handlers ./internal/task/catalog ./internal/mcp/server ./internal/mcp/handlers
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk pnpm --dir apps/web exec vitest run lib/settings-discovery/task-settings-contract.test.ts components/settings/task-settings-reconciliation.test.tsx
rtk pnpm --dir apps/web run typecheck
rtk pnpm --dir apps/web run i18n:check
rtk pnpm --dir apps/web e2e:run --project chromium tests/settings/task-settings-mcp.spec.ts
rtk pnpm --dir apps/web e2e:run --project mobile-chrome tests/settings/mobile-task-settings-mcp.spec.ts
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

Add `TestTaskSettingsParity` and `TestTaskSettingsAuthority` in
`apps/backend/internal/mcp/server/settings_task_settings_test.go`.
Use real registered tools plus domain fixtures, not only direct handler invocation.
Add domain tests for any changed persistence path, including rollback and concurrent updates.
When a SQL query changes, include SQLite/PostgreSQL behavior evidence under the existing domain test conventions.

## E2E scenario

Change a supported task default or priority, observe task details and list state, and reject a lifecycle-field patch.
Use disposable resources and mock providers. Restore user-level settings in cleanup, including after failures.
Run desktop and phone separately with the managed runner and record discovered test counts.

## Files likely touched

- `apps/backend/internal/task/dto/requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/catalog/`
- `apps/backend/internal/mcp/handlers/config_task_handlers.go`
- `apps/backend/internal/mcp/handlers/deferred_launch_prompt.go`
- `apps/web/components/task/`
- `apps/backend/internal/backendapp/services.go` and MCP handler composition.
- `apps/backend/internal/mcp/server/settings_task_settings_test.go` (new).
- `apps/web/lib/settings-discovery/task-settings-contract.test.ts` (new).
- `apps/web/components/settings/task-settings-reconciliation.test.tsx` (new): clean updates, dirty conflicts, own acknowledgements, and late replies.
- `apps/web/lib/settings-discovery/coverage-inventory.ts` and generated snapshots.
- Relevant domain store subscriptions, current editor state, and locale catalogs.
- The two E2E files named in Verification.

## Dependencies

Task 08 completes the preceding slice.
Tasks 00 through 04 establish inventory, schemas, generic tools, and draft-recovery behavior.
The sequential order avoids conflicts in shared composition and generated snapshots.

## Risks

- Task fields can trigger lifecycle effects. A generic patch must not become an alternate move or agent-start API.
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

- Registered permitted task settings with explicit lifecycle and metadata exceptions.
- Preserved workflow-step relationships, ordering boundaries, and task-domain validation.
- Covered task field classification and adapter authorization in the focused backend suite.
