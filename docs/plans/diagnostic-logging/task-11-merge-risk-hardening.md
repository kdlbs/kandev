---
id: "11-merge-risk-hardening"
title: "Merge-risk hardening"
status: completed
wave: 8
depends_on:
  - "01-backend-log-sinks"
  - "02-frontend-error-endpoint"
  - "04-toast-reporting"
  - "08-browser-logs-and-bundle-ui"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 11: Merge-risk hardening

## Confirmed root causes

- The PR head passed lint before `main` added three effective lines to
  `apps/web/lib/types/backend.ts`; the merge result now exceeds the enforced
  600-line limit.
- Request-count limiting and age-only file retention do not bound bytes written
  by authenticated automatic toast reports during one UTC day.
- Automatic toast reports retain URL query and fragment data even though task
  correlation can be derived before redaction.
- IndexedDB pruning materializes and sorts every retained record after each
  persistence batch, creating avoidable allocation and main-thread work.

## Acceptance

- The current `main` merge result passes web lint and typecheck without
  suppressing the line limit.
- The automatic-toast endpoint retains its request-count buckets and also
  enforces exact bounded-body byte buckets per identity and process-wide.
- A daily backend file never grows beyond 256 MiB; entries that would cross the
  bound are dropped without disabling the writer, and the next UTC day accepts
  writes normally.
- Automatic toast reports preserve task-ID derivation but send only URL origin
  and pathname.
- IndexedDB retention uses the existing timestamp index and bounded cursor work
  instead of `getAll()` plus an in-memory sort. No IndexedDB version/schema
  migration is introduced.
- No rendered UI, navigation, touch, scrolling, or responsive behavior changes;
  existing desktop/mobile bundle E2E coverage remains applicable.

## TDD and verification

1. Add failing Go coverage for byte-weighted endpoint rejection and the daily
   file bound.
2. Add failing Vitest coverage for URL redaction and cursor-based retention.
3. Implement the minimum fixes, then run:

```bash
cd apps/backend
go test ./internal/common/logger ./internal/system/frontenderrors
```

```bash
cd apps
pnpm --filter @kandev/web test -- \
  lib/api/domains/frontend-error-log-api.test.ts \
  lib/logger/indexeddb-store.test.ts
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

## Risks and out of scope

- This task does not change the accepted authenticated install-wide backend
  bundle access boundary or restore removed legacy log APIs/configuration.
- The browser persistence change must use the existing IndexedDB schema.
- Diagnostic pressure remains best effort; loss must not block application
  work.

## Verification results

- `go test -race ./internal/common/logger ./internal/system/frontenderrors`:
  32 tests passed.
- Focused Vitest: 10 tests passed across the frontend-error client and
  IndexedDB store.
- Web typecheck and lint: passed.
- CI-style golangci-lint against `origin/main`: passed.
- Public docs: 58 validator tests and all 41 published pages passed.
- `git diff --check`: passed.
