---
id: "09-remove-legacy-diagnostics"
title: "Remove legacy diagnostics"
status: completed
wave: 6
depends_on:
  - "07-diagnostic-bundle-backend"
  - "08-browser-logs-and-bundle-ui"
  - "10-agent-diagnostic-materialization"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 09: Remove legacy diagnostics

## Acceptance

- Backend logging no longer allocates or writes the process-wide structured
  ring buffer, and no product path imports it.
- System Logs list/tail/individual-download APIs and their frontend
  hooks/state/components are removed after the bundle UI has replaced them.
- Dev-only `/api/v1/system/debug/export` is removed; runtime metadata comes
  from bundle manifests.
- Improve Kandev creates an authenticated all-sources browser job. Its
  owner-verified lease endpoint copies the ready/partial ZIP for 24 hours and
  gives the agent that path instead of separate snapshots; bootstrap never
  waits for frontend collection.
- Bootstrap writes a server-owned identity marker; lease authorization checks
  that marker and bundle ownership rather than trusting the submitted temp path.
- `scripts/kandev-logs` no longer calls debug export or tails the isolated
  stderr convention; the focused agent-guidance task can finalize its UX on
  the source-selectable bundle API.

## Verification

```bash
cd apps/backend
go test ./internal/common/logger ./internal/debug ./internal/improvekandev ./internal/system/...
```

```bash
cd apps
pnpm --filter @kandev/web test -- \
  components/improve-kandev-dialog-helpers.test.ts \
  lib/api/domains/improve-kandev-api.test.ts \
  lib/state/slices/system/system-slice.test.ts
```

```bash
rg -n "buffer\\.Default|BufferSnapshot|debug/export|logs/tail|buildLogDownloadUrl|useLogTail" \
  apps/backend apps/web scripts
```

## Files likely touched

- `apps/backend/internal/common/logger/logger.go`
- `apps/backend/internal/common/logger/logger_test.go`
- `apps/backend/internal/common/logger/buffer/`
- `apps/backend/internal/debug/export.go`
- `apps/backend/internal/debug/export_test.go`
- `apps/backend/internal/system/logs/`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/improvekandev/bundle.go`
- `apps/backend/internal/improvekandev/handler.go`
- `apps/backend/internal/improvekandev/*_test.go`
- `apps/web/hooks/domains/system/use-log-files.ts`
- `apps/web/hooks/domains/system/use-log-tail.ts`
- `apps/web/lib/state/slices/system/`
- `apps/web/components/improve-kandev-dialog-helpers.ts`
- `apps/web/components/improve-kandev-dialog-create.tsx`
- `apps/web/lib/api/domains/improve-kandev-api.ts`
- `scripts/kandev-logs`

## Dependencies

- Task 07 supplies the shared bundle service and manifest metadata.
- Task 08 replaces the Logs page and supplies browser capture.
- Task 10 establishes the replacement authenticated agent workflow before the
  debug export disappears.

## Parallelism

Sequential. It removes compatibility paths only after all consumers have
migrated.

## Inputs

- Spec: Agent diagnostics, legacy route removal, Improve Kandev reuse, and
  out-of-scope viewer/query behavior.
- Plan: Legacy-path removal and Improve Kandev migration.
- Existing patterns: logger buffer core, debug export, System Logs
  service/hooks/store, and Improve Kandev temp bundle.

## Risks

- Removal spans Go and TypeScript consumers; source scans must distinguish
  unrelated ring buffers used by agents, terminals, or plugins.
- The new `internal/system/frontenderrors` package is explicitly retained while
  the legacy `internal/system/logs` list/tail/download package is removed.
- Improve Kandev agents need a readable ZIP path and updated instructions to
  extract it before diagnosis.
- The 24-hour Improve Kandev task-context lease must not weaken the public
  bundle API's 15-minute expiry or leak files beyond its existing cleanup
  window.

## Output contract

Report removed APIs/buffer consumers, Improve Kandev migration and expiry
behavior, files changed, exact tests/source scans, blockers/risks, and update
this task plus `plan.md` status in the same conversation.
