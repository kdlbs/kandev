---
id: "02-detect-backend-generation"
title: "Detect changed backend generation"
status: done
wave: 2
depends_on:
  - "01-publish-page-boot-identity"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.2
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.3
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.4
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.9
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.11
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
---

# Task 02: Detect Changed Backend Generation

## Summary

Create the one-way reload coordinator. Signal it after a changed boot ID or an
exact stale-interlock response.

## In scope

- Add the document-local coordinator and subscription contract.
- Check `/api/v1/system/info` after each successful WebSocket connection.
- Ignore failed, missing, equal, and out-of-order identity results.
- Add the stable stale-interlock response code.
- Signal the coordinator from the shared API client for that exact code.
- Keep profile-delete HTTP 409 behavior through the shared response parser.
- Add backend and frontend tests with a TDD red phase.

## Out of scope

- The visible reload alert and locale copy.
- Timer-based polling or WebSocket retry changes.
- Changes to interlock token validation or route scope.

## Acceptance

- A successful connection check latches reload-required only for a changed ID.
- Repeated or late signals cannot clear the state or create duplicate reports.
- The stale-interlock code enters the same state and marks that error handled.
- Other HTTP 403 and connection failures keep their current behavior.

## Verification

```bash
go test ./apps/backend/internal/common/httpmw ./apps/backend/internal/agent/settings/handlers
cd apps && pnpm --filter @kandev/web test -- --run lib/platform/backend-reload-coordinator.test.ts hooks/domains/system/use-backend-generation-guard.test.ts lib/api/client.test.ts app/actions/agents.test.ts
```

## Files likely touched

- `apps/backend/internal/common/httpmw/interim_settings_interlock.go`
- `apps/backend/internal/common/httpmw/interim_settings_interlock_test.go`
- `apps/backend/internal/agent/settings/handlers/interim_settings_interlock_test.go`
- `apps/web/lib/platform/backend-reload-coordinator.ts`
- `apps/web/lib/platform/backend-reload-coordinator.test.ts`
- `apps/web/hooks/domains/system/use-backend-generation-guard.ts`
- `apps/web/hooks/domains/system/use-backend-generation-guard.test.ts`
- `apps/web/lib/api/client.ts`
- `apps/web/lib/api/client.test.ts`
- `apps/web/app/actions/agents.ts`

## Dependencies

- Task 01 supplies the document boot ID.

## Risks

- The connection state can change while an identity request is pending.
- The API client must not classify all HTTP 403 responses as stale documents.
- The profile delete action has a typed conflict result that must not change.

## Parallelism

`sequential`

## Inputs

- Acceptance criteria 001.2, 001.3, 001.4, 001.9, and 001.11.
- System-design sections `Detection flow`, `Stale-interlock fallback`, and
  `Recovery coordinator`.
- Existing shared connection state and `fetchSystemInfo` API.

## Results

Implemented the document-local coordinator, connection recovery guard, typed
interlock code, shared API classification, and profile-delete response
handling. Protected agent-settings presenters suppress only errors marked as
handled by the coordinator.

Verification passed:

- `go test ./internal/common/httpmw ./internal/agent/settings/handlers`: 154
  tests passed.
- Focused coordinator, generation-guard, API-client, action, and agent-settings
  suites: passed.
