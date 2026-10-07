---
id: "01-gate-running-backfill"
title: "Gate running backfill on document visibility"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-HIDDEN-RUNNING-BACKFILL-001
acceptance_criteria:
  - AC-UI-HIDDEN-RUNNING-BACKFILL-001.1
  - AC-UI-HIDDEN-RUNNING-BACKFILL-001.2
  - AC-UI-HIDDEN-RUNNING-BACKFILL-001.3
system_design:
  - ../../specs/ui/system-design/hidden-running-backfill.md
---

# Task 01: Gate Running Backfill

## Summary

Return early from the running backfill tick while the document is hidden.

## In scope

- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-messages.test.ts`
- `apps/web/e2e/tests/chat/hidden-running-backfill.spec.ts`

## Out of scope

- Foreground refresh, other timers, and panel-level visibility.

## Acceptance conditions

- With a running session and fake time, a visible document issues three
  `message.list` reads in 11.2 s and a hidden document issues none.
- The existing session hook suites pass unchanged.
- In a browser with a running session, no `message.list` request is sent
  during 12 s of hidden document, and requests resume once it is visible.

## Verification

From `apps/`:

```bash
pnpm --filter @kandev/web exec vitest run hooks/domains/session/
pnpm --filter @kandev/web typecheck
pnpm --dir web e2e:run --host --shards 1 --project chromium tests/chat/hidden-running-backfill.spec.ts
```

## Results

The hidden-document test failed before the gate and passes after it. 86 test
files and 851 tests pass in `hooks/domains/session/`; typecheck passes. The
Playwright spec failed with the gate reverted (2 requests sent while hidden)
and passes with it.
