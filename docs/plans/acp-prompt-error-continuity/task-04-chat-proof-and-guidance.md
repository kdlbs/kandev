---
id: "04-chat-proof-and-guidance"
title: "Expose error acknowledgement and prove continuity"
status: done
wave: 4
depends_on:
  - "03-task-error-attention"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-004
  - REQ-PLATFORM-TURN-CONTINUITY-005
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-001.3
  - AC-PLATFORM-TURN-CONTINUITY-001.6
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.2
  - AC-PLATFORM-TURN-CONTINUITY-002.4
  - AC-PLATFORM-TURN-CONTINUITY-002.5
  - AC-PLATFORM-TURN-CONTINUITY-004.2
  - AC-PLATFORM-TURN-CONTINUITY-004.3
  - AC-PLATFORM-TURN-CONTINUITY-004.6
  - AC-PLATFORM-TURN-CONTINUITY-004.7
  - AC-PLATFORM-TURN-CONTINUITY-005.1
  - AC-PLATFORM-TURN-CONTINUITY-005.2
  - AC-PLATFORM-TURN-CONTINUITY-005.3
  - AC-PLATFORM-TURN-CONTINUITY-005.4
  - AC-PLATFORM-TURN-CONTINUITY-005.5
  - AC-PLATFORM-TURN-CONTINUITY-005.6
  - AC-PLATFORM-TURN-CONTINUITY-005.7
  - AC-PLATFORM-TURN-CONTINUITY-005.8
system_design:
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 04: Expose error acknowledgement and prove continuity

## Summary

Prove generic ACP error continuity through desktop and phone Chat using the real backend and subprocess transport.
Add localized acknowledgement through Chat and task actions, and update public guidance after the behavior works.

## In scope

- Add deterministic generic/storage/auth/quota/model error scenarios without transient keywords or mock capacity markers.
- Run fixtures under production OpenCode and another ACP identity through `KANDEV_MOCK_PROVIDERS`.
- Reuse trace evidence for process, connection, native session, initialize/new/load/resume, prompt, and side-effect counts.
- Prove a completed effect occurs once and a distinct later user prompt succeeds on the original runtime.
- Cover repeat failures, one error per failed turn, reload, pagination, and an additional viewer.
- Cover actual runtime death after a retained error, using a real crash or pipe close.
- Inventory `/transport-lost` consumers. Keep its remote-error semantics distinct from actual disconnect evidence, and migrate only affected loss fixtures.
- Add desktop and phone Playwright coverage through the normal composer and touch Send.
- Keep phone details wrapped within Chat with no horizontal overflow, unchanged scroll ownership, and existing touch/safe-area behavior.
- Extend the existing ActionMessage with stamped Dismiss and keep runtime recovery projection separate.
- Update the explanation/recovery section in `docs/public/sessions-and-review.md` after implementation.
  Reconcile related wording in `tasks-and-workflows.md` only if it describes the changed lifetime boundary.
- Update mock fixture guidance when commands or semantics change.

- Render the task error icon from the new nonblocking summary field, without session transcript subscriptions.
- Add a current-error inline Dismiss action and task-menu Dismiss error action using the same stamped backend API.
- Expose the action from desktop right-click/overflow. Add a new visible per-row phone task-menu trigger in `mobile/session-task-switcher-sheet.tsx`.
  Reuse shared touch menu primitives; this sheet does not already contain that trigger.
- Quick Chat receives inline Dismiss and same-session prompt acknowledgement. Do not add normal sidebar rows or a task menu for Quick Chat.
- If runtime and turn errors coexist, keep separate controls. Label the new action Dismiss turn error so clearing it does not promise runtime recovery.
- Prove focus does not clear the icon. New input needs a client-observed same-session stamp; cross-session, rejected, and duplicate sends preserve attention.
- Prove an entry queued before failure cannot clear the new badge on automatic drain, edit, retry, or Send now.
- Carry `observed_turn_error_stamp` through the shared direct, queued, steer, Quick Chat, and phone request construction paths.
- Prove stale menu/close actions preserve a newer sibling failure, and independent runtime/pending indicators remain visible.
- Assert no false successful-turn or needs-answer notification, and keep read/unread state independent of the badge.
- Add API, summary/row selector, acknowledgement hook, and menu/action tests.
- Localize new labels, accessible names, and failure feedback in all required catalogs.

## Out of scope

- New navigation, layout systems, model selectors, or standalone notification/inbox features.
- Retrofitting historical runtime errors as live sessions or inferring current liveness from persisted metadata.
- Broad browser suites or additional generic QA/review passes.

## Acceptance

1. Desktop and phone show a sanitized inline error and accept a new, distinguishable user response on the same runtime.
   Repeat failures and reload retain unique historical entries without extra initialization or automatic replay.
2. Close and task-menu dismissal acknowledge the observed error. A new prompt requires client-observed same-session acknowledgement evidence.
   Focus preserves attention. Newer errors and unrelated runtime/pending indicators remain protected.
   A later genuine crash exposes existing recovery controls. Phone controls remain usable within the viewport.
3. Public guidance explains the failed-turn/runtime distinction and unchanged automatic-recovery limits.
   Planned and actual source coverage and final results are synchronized with this package.

## ASCII UI preview

Use [UI-01, UI-02, and UI-03 in the plan](plan.md#ascii-ui-preview).

```text
UI-01: desktop and phone
  [Transcript]
  Sidebar: [!] Task title
  [!] <sanitized error> [Technical details] [Dismiss]
  [Follow-up message                ] [Send]

UI-03: desktop context menu / visible phone More options
  ...existing task actions...
  Dismiss error

UI-02: after a later real crash
  [!] <earlier turn error>
  [Runtime recovery]
  [Resume session] [Start fresh session]
```

The existing chat body owns transcript scrolling. The composer retains its fixed region and draft.
On phone, details wrap and controls retain touch sizing and safe-area clearance.
Reuse the existing task menu surface. Phone controls retain at least 44px hit targets.
Focusing preserves the badge. Dismiss acknowledges it and preserves readable history.
Spacing and diagnostic wording are illustrative.
Mappings: AC-PLATFORM-TURN-CONTINUITY-002.1, 002.4, 002.5, 004.3, 004.7, and 005.1-.8.

## Verification

Use a defect-specific rendered assertion for RED when the fixture can exercise the baseline behavior.
An existing adapter RED can own the defect if browser scaffolding depends on the corrected SDK; record that reason.
Run from the repository root. Install dependencies once if this worktree has no workspace install.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/mock-agent -run 'PromptError|TransportLost|Capacity|Scenario' -count=1)
(cd apps/web && pnpm exec vitest run lib/task-status-summary.test.ts components/task/task-session-sidebar-item.test.ts components/task/task-switcher-context-menu.test.tsx hooks/domains/session/use-session-prompts.test.ts hooks/use-task-turn-error.test.tsx lib/kanban/task-actions-menu-entries.test.ts lib/api/domains/task-turn-error-api.test.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint e2e/helpers/acp-prompt-error-continuity.ts e2e/tests/session/acp-prompt-error-continuity.spec.ts e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/acp-prompt-error-continuity.spec.ts tests/session/transient-turn-runtime-continuity.spec.ts tests/session/opencode-capacity-retry.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-acp-prompt-error-continuity.spec.ts tests/session/mobile-transient-turn-runtime-continuity.spec.ts tests/session/mobile-opencode-capacity-retry.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the two managed browser commands sequentially with fresh builds.
Add any existing browser files changed during the transport-loss fixture migration to the documented focused commands.
Do not pass worker overrides or overlap full suites.
Run the plan's documentation-coverage preflight for actual and planned paths.
Run the Traditional Chinese/pseudo generators for the new catalog keys before the required i18n check.
Include every production TypeScript file touched by the acknowledgement UI in scoped ESLint.
If implementation adds another production hook or state helper, include its targeted tests in the verification block and run them.
Before package handoff, verify untracked work orders with `git status --short -- docs/plans/acp-prompt-error-continuity`.

## Files likely touched

- `apps/web/lib/types/task-status-summary.ts`, `lib/task-status-summary.ts`, and adjacent summary tests.
- New `apps/web/lib/api/domains/task-turn-error-api.ts` and its test.
- `apps/web/components/task/task-session-sidebar-item.ts` and task-row selector tests.
- `apps/web/components/task/task-switcher-context-menu-items.tsx`, shared task action entries, and their tests.
- `apps/web/hooks/use-task-actions-menu.ts`, `lib/kanban/task-actions-menu-entries.ts`, and existing task menu triggers.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` for the new visible per-row phone trigger.
- `apps/web/components/quick-chat/quick-chat-session-view.tsx`, `quick-chat-content.tsx`, and their inline-error/composer tests.
- `apps/web/lib/api/domains/session-api.ts` and `queue-api.ts` for the optional observation field and replay-compatible request types.
- `apps/web/hooks/domains/session/use-queue-admission.ts` and queue retry/send-now request construction.
- New `apps/web/hooks/use-task-turn-error.ts` and its test, shared by inline and task-menu actions.
- `apps/web/hooks/domains/session/use-session-prompts.ts` and `use-queue.ts` only for accepted-input acknowledgement presentation.
- Required locale catalogs and their generated Traditional Chinese/pseudo values.

- New `apps/backend/cmd/mock-agent/prompt_error_continuity.go` and focused tests if its scenario logic needs coverage.
- `apps/backend/cmd/mock-agent/handler.go` and `AGENTS.md` for dispatch and accurate fixture semantics.
- New `apps/web/e2e/helpers/acp-prompt-error-continuity.ts`.
- New `apps/web/e2e/tests/session/acp-prompt-error-continuity.spec.ts` and `mobile-acp-prompt-error-continuity.spec.ts`.
- Existing `apps/web/e2e/helpers/transient-turn-runtime-continuity.ts` and `opencode-capacity-retry.ts`, only for reusable trace/fixture behavior.
- `apps/web/components/task/chat/messages/action-message-recovery-history.tsx` and adjacent tests, only if generic error projection needs correction.
- `apps/web/components/task/chat/messages/action-message.tsx`, only if the rendered regression exposes a defect.
- `docs/public/sessions-and-review.md`, and `tasks-and-workflows.md` only for related current wording.
- This plan, work orders, and the paired draft design for final traceability and lifecycle status.

## Dependencies

Task 03 and the completed SDK/runtime prerequisites. Preserve existing capacity and genuine runtime recovery behavior.

## Risks

- Mock Data can imitate a transport error without killing ACP. Tests must observe actual death when that is their premise.
- Persisted earlier responses can make a follow-up test pass falsely. Capture baseline counts and wait for a new turn-specific marker.
- Browser reload must not reconstruct live authority from history or retire genuine runtime recovery controls.

## Parallelism

`sequential`

## Inputs

- [Design: failed-turn history and desktop/phone Chat](../../specs/platform/system-design/acp-prompt-error-continuity.md#failed-turn-history-and-desktopphone-chat)
- [Plan: UI previews and E2E matrix](plan.md#ascii-ui-preview)
- Existing desktop/phone retained-capacity scenarios and `SessionPage` composer/recovery helpers.
- `/mobile-parity`, `/e2e`, `/docs-maintainer`, and the scoped frontend and mock-agent guides.

## Results

Done. Desktop and phone surfaces show the retained error, an inline Dismiss
control, and a task-menu Dismiss turn error action. Task rows render the durable
turn-error badge without subscribing to transcripts. Accepted manual prompts
carry the client-observed same-session stamp.

Added:

- `task_error`-independent `turn_error` selectors and row precedence (runtime
  error first, then turn attention).
- `useTaskTurnError` and `task-turn-error-api` shared by the inline alert and the
  task menu. Failures keep the badge and show localized feedback.
- Phone-visible per-row task actions trigger with coarse-pointer 44px targets.
- Generic `/prompt-error`, `-auth`, `-quota`, `-model`, and `-empty` mock-agent
  scenarios with no capacity marker or transient keyword.
- Public guidance in `docs/public/sessions-and-review.md` and
  `docs/public/tasks-and-workflows.md`.
- Desktop and phone Playwright specs with a shared
  `e2e/helpers/acp-prompt-error-continuity.ts` fixture.
- Task-backed Quick Chat now reads retained attention from the bounded live
  summary cache, and hydrates it from `GET /api/v1/tasks/:id` when the ephemeral
  task is absent from board/overview collections. Inline dismissal and
  same-session follow-up acknowledgement work without a new sidebar row.
- Accepted direct/queued acknowledgement failures surface localized nonfatal
  feedback while keeping the prompt accepted and the badge visible.
- Runtime-loss browser scenarios now use a genuine `/crash` subprocess exit;
  `/transport-lost` remains only the response-shaped continuity fixture and its
  documentation no longer claims recovery controls.
- New translations in all required catalogs, including generated Traditional
  Chinese and pseudo catalogs.

Verification completed:

- `pnpm run typecheck`: passed.
- `pnpm --filter @kandev/web build:vite`: passed.
- `pnpm run i18n:check`: passed after regenerating pseudo and Traditional Chinese
  catalogs.
- Focused Vitest: `lib/task-status-summary.test.ts`,
  `lib/api/domains/task-turn-error-api.test.ts`,
  `hooks/use-task-turn-error.test.tsx`, sidebar/sheet item tests,
  `action-message.test.tsx`, and `use-message-handler.test.ts`: passed.
- `pnpm exec eslint` on the new E2E helper and specs: passed.
- `node scripts/validate-public-docs.mjs`: passed.
- Desktop Playwright (`chromium`): 4 passed, including the task-backed Quick
  Chat case with empty board/overview collections. Phone Playwright
  (`mobile-chrome`): 2 passed. Both ran sequentially through the managed runner
  with fresh builds; see the plan verification results.
- Retargeted runtime-loss spec: 2 passed on `chromium`. The Kubernetes
  `containers` project was updated to `/crash` but was not run in this
  environment because no container daemon/Kind cluster is provisioned.
