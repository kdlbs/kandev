---
id: "01-repair-rejection-feedback"
title: "Repair MCP rejection feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.9
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 01: Repair MCP Rejection Feedback

## Summary and root cause

Separate ordinary bearer rejection from stale-token recovery and give MCP
selection errors one presentation owner. The backend's combined rejection
condition returns the stale-token code for bearer requests. The client
correctly reacts to that code, but MCP presenters ignore its handled marker.
The session component also leaves an optimistic checkbox selected after a
rejected write. Preserve the last authoritative selection on write failure.

## Scope and files

- `apps/backend/internal/common/httpmw/interim_settings_interlock.go` and its
  existing test: reject bearer credentials first with HTTP 403 and ordinary
  `forbidden` code; keep the stable stale-interlock code for missing/wrong
  browser tokens. Keep constant-time comparison and fail-closed behavior.
- `apps/web/lib/api/client.test.ts`: cover ordinary 403 without reload and the
  existing typed stale-interlock handled/reload contract. Change client code
  only if the regression exposes a gap; its current distinction is appropriate.
- `apps/web/components/task/mcp-session-selector.tsx`,
  `apps/web/app/settings/agents/[agentId]/profile-mcp-selection-card.tsx`, and
  `apps/web/components/settings/repository-mcp-selection.tsx`: suppress a local
  error when `isHandledApiError` says the shared recovery surface owns it.
- `apps/web/components/task/task-session-mcp-settings.tsx` and relevant
  selection editor callers: on rejected writes, restore the authoritative
  selected IDs, keep desired/applied revisions unchanged, and preserve existing
  request/context race guards.
- Add `apps/web/hooks/domains/workspace/use-mcp-selection-editor.test.ts` for
  rejected-write authority and context changes. Use the shipped hook test
  patterns; test state behavior, not JSX structure.
- Add a localized MCP-selection save error where needed in
  `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/settings.json`.
  Generate the Traditional Chinese pair with the existing conversion script.
- Extend the desktop and phone task MCP E2E files with ordinary 403 and typed
  stale-token responses at the real API-client boundary.

No auth bypass, forwarding configuration change, catalog mutation, provider
reconfiguration change, or disclosure redesign in this work order.

## Acceptance

1. Every bearer request is rejected, including mixed-case bearer schemes with
   a valid token. Its response never triggers restart recovery. A matching
   non-bearer browser token permits the normal protected handler.
2. A genuine stale token triggers one global reload surface without duplicate
   local MCP errors. Ordinary rejected selections show a localized local error
   and retain prior saved IDs and desired/applied revisions.
3. Desktop and phone evidence distinguishes both errors; normal idle-session
   additions still save and report their authoritative application state.

## Regression sequence

Before implementation, update the existing middleware table to expect ordinary
`forbidden` for bearer cases and stable stale-token code for missing/wrong tokens.
Add valid-token and wrong-token bearer cases, including mixed-case schemes, and
keep the matching non-bearer positive control. The new classification assertion
must fail against the current combined condition.

Add API-client ordinary-403 and genuine-stale tests. Add hook/caller rejection
tests that verify no saved-state promotion and no cross-context rollback.
Add Playwright cases named `keeps ordinary MCP authorization failures local`
and `shows one recovery surface for stale MCP selections` to each task MCP
suite. Route only the selection mutation under test; retain normal boot/reads.
Assert error surfaces and persisted selections, not just request status.

## ASCII UI preview

UI-02 from [the plan](plan.md#ui-02-rejected-selection-write), idle task session:

```text
Ordinary 403, desktop inline / phone sheet:
  Could not save the MCP selection.
  [ ] Issue tracker   (previous saved selection)

Genuine stale token:
  [Reload required ... Reload page]
  No duplicate local save error
```

Copy is illustrative and localized. On phones the error remains within the
selection surface; the shared reload alert retains its existing placement.

## Exact verification commands

Run each group from the named directory; execute browser projects sequentially.

From `apps/backend`:

```bash
go test ./internal/common/httpmw ./internal/agent/settings/handlers
```

From `apps/web`:

```bash
pnpm exec vitest run lib/api/client.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts
pnpm run typecheck
pnpm run i18n:zh-hant
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --project chromium -- tests/task/task-mcp-selection.spec.ts
pnpm e2e:run --host --project mobile-chrome -- tests/task/mobile-task-mcp-selection.spec.ts
```

Also run targeted ESLint on every changed TS/TSX path using the repository
configuration. After success, rebuild the owned pod-local test runtime
while retaining its user-test database and leaving :9998 untouched. Verify direct save success and forwarded rejection with
no false restart signal. Do not assume the forwarding service has been repaired.

## Dependencies and results

Uses existing shared `ApiError` handling and reload coordinator. No dependency
on Task 02. Sequential work; no delegation. Completed: Go middleware/handler suites passed;
17 API-client/editor tests passed; desktop task suite passed 3 tests and phone
suite passed 4 tests. Typecheck, targeted ESLint, i18n check, and new-code ratchet
passed. RED evidence reproduced bearer misclassification, duplicate stale error,
and rejected-write checkbox promotion. Session selections now render directly
from authoritative editor responses, so no asynchronous UI rollback can affect
a replacement session. No production change to the shared API client was needed.
Traditional Chinese and pseudo generation ran; unrelated converter changes were
removed. After Task 02, the final combined run passed 32 unit tests and the same
3 desktop/4 phone browser tests. Tested artifacts are deployed on :48593 with
the database retained. Fresh forwarded rejection returns `forbidden`, preserves
the selection, and shows no reload alert; the forwarding credential prerequisite
remains. See the companion plan for final deployment and verification results.
