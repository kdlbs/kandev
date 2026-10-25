---
id: "09-promotion-release-management"
title: "Canvas release governance"
status: done
wave: 9
depends_on:
  - "08-agent-canvas-authoring"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-002
  - REQ-CANVASES-AGENT-WEB-APPS-003
  - REQ-CANVASES-AGENT-WEB-APPS-007
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-002.3
  - AC-CANVASES-AGENT-WEB-APPS-002.4
  - AC-CANVASES-AGENT-WEB-APPS-003.1
  - AC-CANVASES-AGENT-WEB-APPS-003.2
  - AC-CANVASES-AGENT-WEB-APPS-003.3
  - AC-CANVASES-AGENT-WEB-APPS-003.4
  - AC-CANVASES-AGENT-WEB-APPS-003.5
  - AC-CANVASES-AGENT-WEB-APPS-007.4
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 09: Canvas release governance

## Summary

Add human promotion, permission review, release history, rollback, archive,
restore, and removal APIs. Keep scope, grants, and release activation atomic.

## In scope

- Add promotion preview and confirmation services.
- Show task and workspace scope plus every requested permission.
- Change instance scope and grants in one transaction.
- Add pending-release approval and rejection.
- Retain one prior valid release and at most one pending release.
- Add release history, rollback, archive, restore, and remove handlers.
- Prevent agents from using human lifecycle operations.
- Add authorization, retention, transaction, and rollback tests.

## Out of scope

- Quick Chat editing and frontend management surfaces.

## Acceptance

- Promotion changes scope and grants together or changes neither.
- A permission increase keeps the current release active until approval.
- Rollback selects the retained prior release and rechecks current grants.

## Verification

```bash
cd apps/backend && go test ./internal/canvas/... ./internal/plugins/... ./internal/backendapp/...
cd apps && pnpm --filter @kandev/web test -- lib/api/domains/canvas-api
```

## Files likely touched

- `apps/backend/internal/canvas/promotion.go`
- `apps/backend/internal/canvas/releases.go`
- `apps/backend/internal/canvas/handlers.go`
- `apps/backend/internal/plugins/instances/**`
- `apps/web/lib/api/domains/canvas-api.ts`
- `apps/web/lib/api/domains/canvas-api.test.ts`
- `apps/web/lib/types/canvas.ts`

## Dependencies

- Task 08 provides active task canvases and published releases.

## Risks

- Permission approval can activate a release before the transaction commits.
- Rollback can restore authority that the user revoked.
- Restore can exceed canvas or storage limits without shared admission.

## Parallelism

`sequential`

## Inputs

- Promotion, permissions, publish, rollback, HTTP API, and authorization
  sections.
- Existing plugin permission and task authorization tests.

## Results

Implemented human-only promotion and permission review, atomic metadata, scope,
and grant changes, pending-release approval and rejection, release history,
rollback, archive, restore, removal, retention, and authorization checks.

Verification:

- `go test ./internal/canvas/... ./internal/plugins/... ./internal/backendapp/... -count=1` — passed.
- Focused canvas API tests — passed.
