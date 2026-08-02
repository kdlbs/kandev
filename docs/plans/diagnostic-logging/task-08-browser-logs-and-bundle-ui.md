---
id: "08-browser-logs-and-bundle-ui"
title: "Browser logs and bundle UI"
status: completed
wave: 4
depends_on:
  - "04-toast-reporting"
  - "07-diagnostic-bundle-backend"
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 08: Browser logs and bundle UI

## Acceptance

- Console debug/info/warn/error calls, uncaught errors, and unhandled
  rejections retain normal browser behavior while entering a bounded three-day
  IndexedDB history with URL and recognized-route task ID.
- Retention enforces 64 KiB per entry and 10,000-entry/20 MiB per browser
  profile limits. IndexedDB failure degrades to the 500-entry memory buffer.
- Entries are partitioned by authenticated Kandev identity; account switches
  in one browser profile cannot include the prior account's frontend history.
- Every tab handles identity-scoped `system.logs.capture_requested` and uploads
  a sequential bounded snapshot with a random per-tab `capture_stream_id`,
  without logging its own failures or causing console recursion.
- The final `done` chunk includes at most 8 KiB of loss, persistence, storage,
  and truncation metadata for the manifest.
- Interception creates reference-free previews only: at most 20 arguments,
  bounded primitives/Error fields, and non-traversing type descriptors for
  other values. It never retains arbitrary caller objects.
- The staging queue is capped at 500 entries/2 MiB. IndexedDB work drains at
  most 50 entries/256 KiB after 250 ms or idle time with a one-second timeout;
  full staging sheds debug/info before warn/error and records loss.
- System Logs renders one clear combined-download workflow with sensitive-data
  disclosure and collecting/preparing/partial/busy/error feedback, including
  `Retry-After` handling. Tail, copy, refresh, file table, and individual
  download UI are absent.
- Desktop and phone provide the same capability. The phone action is
  touch-accessible, uses the page as its single scroll owner, and has no
  horizontal overflow.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  lib/logger/buffer.test.ts \
  lib/logger/intercept.test.ts \
  lib/logger/indexeddb-store.test.ts \
  lib/api/domains/system-api.test.ts \
  components/log-buffer-bridge.test.tsx \
  components/settings/system/log-viewer.test.tsx
```

```bash
cd apps/web
pnpm run typecheck
```

```bash
cd apps/web
pnpm e2e:run e2e/tests/system/logs-page.spec.ts \
  e2e/tests/system/mobile-logs-bundle.spec.ts
```

## Files likely touched

- `apps/web/lib/logger/buffer.ts`
- `apps/web/lib/logger/buffer.test.ts`
- `apps/web/lib/logger/intercept.ts`
- `apps/web/lib/logger/intercept.test.ts`
- `apps/web/lib/logger/indexeddb-store.ts`
- `apps/web/lib/logger/indexeddb-store.test.ts`
- `apps/web/components/log-buffer-bridge.tsx`
- `apps/web/components/log-buffer-bridge.test.tsx`
- `apps/web/lib/ws/handlers/system-events.ts`
- `apps/web/lib/ws/handlers/system-events.test.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/components/settings/system/log-viewer.tsx`
- `apps/web/components/settings/system/log-viewer.test.tsx`
- `apps/web/app/settings/system/logs/page.tsx`
- `apps/web/e2e/tests/system/logs-page.spec.ts`
- `apps/web/e2e/tests/system/mobile-logs-bundle.spec.ts`

## Dependencies

- Task 04 provides shared route/task-ID extraction and preserves toast behavior.
- Task 07 defines the WS capture notification, chunk upload, job status, and
  download contracts.

## Parallelism

Sequential after Tasks 04 and 07 because it implements both approved contracts.

## Inputs

- Spec: Browser console history, System Logs page, bundle APIs, failure modes,
  persistence, desktop/mobile scenarios.
- Plan: Browser console retention and capture; System Logs user workflow.
- Frontend guidance: API calls through domain clients/hooks, no direct component
  fetches, shadcn imports, and explicit sensitive-data copy.
- Mobile guidance: one focal action, ≥44px touch target, shared business logic,
  one scroll owner, and `mobile-chrome` outcome coverage.
- Existing patterns: console interceptor/buffer, log page action feedback,
  SystemPageShell, WS handler registry, and Improve Kandev frontend upload.

## Risks

- Serialization must handle cycles, DOM/host objects, getters, large values,
  quota failures, and unavailable IndexedDB without breaking console calls.
- Object preview must not invoke getters or retain references; hostile proxies
  must fail closed to a generic descriptor within the fixed argument cap.
- Slow or blocked IndexedDB must be testably unable to delay console methods,
  rendering, or the WebSocket event loop.
- Capture failures must not call intercepted console methods.
- Multiple tabs share one browser ID and store; duplicate responses must remain
  safe even when they race.
- Identity partitioning must fail closed while auth state is unavailable or
  changing; it cannot fall back to returning every local partition.
- Download E2E must verify user-visible behavior and ZIP contents against a
  freshly rebuilt production bundle.

## Output contract

Report local storage bounds/fallback, WS upload behavior, desktop/mobile UI
contract, files changed, exact unit/typecheck/E2E results, blockers/risks, and
update this task plus `plan.md` status in the same conversation.
