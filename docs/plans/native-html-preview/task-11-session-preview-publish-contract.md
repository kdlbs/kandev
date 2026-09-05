---
id: "11-session-preview-publish-contract"
title: "Add the session preview publish contract"
status: done
wave: 2
depends_on:
  - "10-agentctl-workspace-preview-server"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 11: Add the session preview publish contract

## Summary

Expose the agentctl publish operation through an authorized task-session API.
Add the frontend client that obtains a session port-proxy URL.

## In scope

- Agentctl client request and response methods.
- Session-scoped backend route, authorization, readiness checks, 5 MiB body
  bound, validation, forwarding, and error mapping.
- Frontend domain API types, request helper, and proxy URL construction.
- Backend and frontend tests for success, authorization, limits, invalid agentctl
  responses, unavailable sessions, and retryable failures.

## Out of scope

- Toolbar, Browser panel, and mobile rendering changes.
- A new gateway route or direct browser-to-agentctl access.

## Acceptance

- An authorized live session can publish `{repo, path, content}` and receive a
  validated `{port, path, version}` result.
- Unauthorized, stopped, malformed, and oversized requests fail without storing
  source content in the backend.
- The frontend produces the same session proxy URL shape as existing
  development-server Browser panels.

## Verification

```bash
cd apps/backend
go test ./internal/agent/runtime/agentctl ./internal/task/handlers
cd ../web
pnpm exec vitest run lib/api/domains/process-api.test.ts
pnpm run typecheck
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/task/handlers/process_handlers.go`
- Focused backend handler and client tests.
- `apps/web/lib/api/domains/process-api.ts`
- `apps/web/lib/api/domains/process-api.test.ts`

## Dependencies

Task 10 defines the agentctl route and response contract.

## Risks

- Generic error translation can hide whether retry requires restarting agentctl
  or only republishing.
- Duplicating existing proxy URL logic can produce inconsistent remote-executor
  behavior.

## Parallelism

`sequential`

## Inputs

- Acceptance criteria `.2`, `.5`, `.6`, and `.10`.
- Browser-to-backend and backend-to-agentctl contracts in the system design.

## Results

Implemented the authorized session publish path.

- Added agentctl client and lifecycle forwarding for current HTML buffers.
- Added session authorization, readiness, body bounds, validation, and stable
  error mapping in the task-session handler.
- Added the frontend process API and versioned session port-proxy URL builder.
- Added backend and frontend success, authorization, limit, and failure tests.

Verification:

- `go test ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/task/handlers` passed.
- `pnpm exec vitest run lib/api/domains/process-api.test.ts` passed.
- `pnpm run typecheck` passed.
