---
id: "02-retain-quick-chat"
title: "Retained Quick Chat setup"
status: in_progress
wave: 2
depends_on: ["01-auth-warning"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.5
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Retained Quick Chat setup

## Summary and scope

Keep persisted Quick Chats after synchronous/asynchronous setup failures and when
a start response arrives after navigation. Render a retained session error or,
when no session exists, an inline setup error with preserved form selections.
Exclude provider authentication changes, new global HTTP behavior and retention-policy changes.

## Acceptance

- Post-allocation launch errors retain task/session and safe error through list, boot, reconnect and reload; pre-session errors keep the setup form and inline retry.
- Navigation and delayed WS/HTTP response ordering never delete a created chat or steal newer focus; explicit deletion remains authoritative.
- Failed conversations remain inspectable without automatic retry; existing guarded recovery reuses identity on desktop and phone.

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- New `apps/backend/internal/task/handlers/quick_chat_failure_retention_test.go`
- Existing task service list and error projection helpers, only where needed for retained-session handling
- `apps/web/lib/api/domains/workspace-api.ts` and tests for typed retained errors
- `apps/web/components/quick-chat/use-quick-chat-modal.ts` and its tests
- `apps/web/components/quick-chat/quick-chat-modal.tsx`, `quick-chat-setup.tsx`, `quick-chat-session-view.tsx` and tests
- `apps/web/lib/state/slices/ui/quick-chat-sync.ts` and tests if late-response reconciliation needs adjustment
- `apps/web/hooks/use-quick-chat-resync.test.ts`, `apps/web/lib/ws/handlers/tasks-quick-chat.test.ts`
- `apps/web/src/locales/` for localized inline errors/retry copy
- `apps/web/e2e/tests/chat/setup-recovery.spec.ts`, `mobile-setup-recovery.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Verification

Add `TestQuickChatFailureRetention` subtests for post-allocation failure with retained
identity, safe persisted errors, pre-session cleanup, list restoration and ownership.
Add hook regression for a delayed creation response following reset; assert no DELETE.
Add active-error/no-auto-resume and explicit retry identity assertions.
From repo root, run red before implementation and green after:

```bash
(cd apps/backend && go test ./internal/task/handlers -run 'TestQuickChatFailureRetention|TestHTTPListQuickChatSessions' -count=1)
(cd apps/web && pnpm exec vitest run components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/quick-chat-setup.test.tsx components/quick-chat/quick-chat-session-view.test.tsx lib/state/slices/ui/quick-chat-sync.test.ts hooks/use-quick-chat-resync.test.ts lib/ws/handlers/tasks-quick-chat.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/setup-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-setup-recovery.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add any changed API/service test files to the exact commands before recording results.
E2E must use managed fixtures and delay the POST response after its real persisted
identity has arrived over WS. Navigate during the delay, release it, and prove the
chat survives. Also cover real injected launch failure, reload/reconnect, explicit
retry, explicit delete, and 44px phone actions without horizontal overflow.

## Risks

Keep error classification sanitized. Do not create a second chat on retained-session
retry or resurrect an explicitly deleted chat. Preserve stopped-session auto-resume
when no active launch error exists. The exact live deletion gesture is not confirmed.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-02: Retained Quick Chat error

Entry: Start chat, then a genuine setup failure. Current reported behavior loses
the created chat; the pre-session request catch exposes only a toast.

Desktop, persisted session:

```text
+--------------------------------------------------+
| [Cursor chat !] [Other chat] [+]               [x] |
| Environment setup failed                         |
| Safe failure details                             |
| [Retry]                                          |
|                                                  |
| Conversation history / preparation details        |
+--------------------------------------------------+
```

Phone, existing full-height Quick Chat surface:

```text
+----------------------------+
| Quick Chat           [Close]|
| [Cursor chat !] [Other chat]|
|----------------------------|
| Environment setup failed   |
| Safe failure details       |
| [Retry]                    |
|                            |
| Scrollable conversation    |
| and preparation details    |
|----------------------------|
| Existing composer / state  |
+----------------------------+
```

Pre-session form failure, both surfaces:

```text
Agent profile: [Cursor         v]
Repositories:  [retained choices]
[!] Could not start chat. Safe reason.
                        [Cancel] [Retry]
```

Structural requirements: retain the selected surface on failure, error text and
valid recovery controls in the existing content region, explicit tab deletion
through the existing confirmation, and no new nested overlay. Existing full-height
phone shell owns safe-area and viewport constraints; content scrolls internally.
Labels and spacing are illustrative and localized. Retry is shown only when the
existing error contract permits it. Maps to all ACs of
REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.

## Dependencies

Task 01; shared E2E scenario files are extended sequentially.

## Parallelism

`sequential`

## Inputs

Read the linked requirement/design amendment, scoped AGENTS.md, existing tests,
and the evidence in plan.md. Follow /tdd, /mobile-parity and /e2e during implementation.

## Results

- Updated `httpStartQuickChat` in `apps/backend/internal/task/handlers/task_http_handlers.go` to retain created ephemeral tasks when a session was allocated and return `task_id` and `session_id` in the 500 error response. Pre-session errors continue to delete unstarted tasks.
- Added `apps/backend/internal/task/handlers/quick_chat_failure_retention_test.go` covering post-allocation failure retention, pre-session rollback, and list restoration.
- Added `getQuickChatRetainedSessionFromError` in `apps/web/lib/api/domains/workspace-api.ts`.
- Updated `useAgentSelection` and `useQuickChatModal` in `apps/web/components/quick-chat/use-quick-chat-modal.ts` to upsert stale responses without deleting, open retained sessions on failure, and track `setupError` for pre-session failures.
- Updated `QuickChatSetup` in `apps/web/components/quick-chat/quick-chat-setup.tsx` to render inline error banners and a "Retry" start button.
- Updated `QuickChatSessionView` in `apps/web/components/quick-chat/quick-chat-session-view.tsx` and `useSessionResumption` in `apps/web/hooks/domains/session/use-session-resumption.ts` to prevent auto-resume when an active launch error is present.
- Updated `docs/public/tasks-and-workflows.md` with Quick Chat failure retention and setup recovery documentation.
- Added unit and E2E tests across `use-quick-chat-modal.test.ts`, `quick-chat-setup.test.tsx`, `quick-chat-session-view.test.tsx`, `setup-recovery.spec.ts`, and `mobile-setup-recovery.spec.ts` (all passed).

## Review validation follow-up

Review corrections and focused unit checks are complete; see the
[local review record](plan.md#local-review-and-corrections-2026-10-02) for the exact
scope, regressions, results, and Chromium sandbox blocker. Earlier E2E counts
above are historical implementation results, not validation of the revised
working tree. Rendered verification subsequently passed on desktop and phone;
see the plan publication-validation follow-up for commands and fixture corrections.
