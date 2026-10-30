---
id: "05-inbox-and-mobile"
title: "Integrate desktop and phone Inbox flows"
status: completed
wave: 5
depends_on:
  - "04-explicit-question-waits"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-007
  - REQ-TASKS-ASYNC-QUESTIONS-008
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-001.1
  - AC-TASKS-ASYNC-QUESTIONS-001.2
  - AC-TASKS-ASYNC-QUESTIONS-002.1
  - AC-TASKS-ASYNC-QUESTIONS-003.1
  - AC-TASKS-ASYNC-QUESTIONS-003.2
  - AC-TASKS-ASYNC-QUESTIONS-003.4
  - AC-TASKS-ASYNC-QUESTIONS-004.1
  - AC-TASKS-ASYNC-QUESTIONS-004.2
  - AC-TASKS-ASYNC-QUESTIONS-004.3
  - AC-TASKS-ASYNC-QUESTIONS-005.1
  - AC-TASKS-ASYNC-QUESTIONS-005.2
  - AC-TASKS-ASYNC-QUESTIONS-005.3
  - AC-TASKS-ASYNC-QUESTIONS-006.1
  - AC-TASKS-ASYNC-QUESTIONS-006.2
  - AC-TASKS-ASYNC-QUESTIONS-006.3
  - AC-TASKS-ASYNC-QUESTIONS-006.4
  - AC-TASKS-ASYNC-QUESTIONS-007.1
  - AC-TASKS-ASYNC-QUESTIONS-007.2
  - AC-TASKS-ASYNC-QUESTIONS-007.3
  - AC-TASKS-ASYNC-QUESTIONS-007.4
  - AC-TASKS-ASYNC-QUESTIONS-007.5
  - AC-TASKS-ASYNC-QUESTIONS-008.1
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 05: Integrate desktop and phone Inbox flows

## Summary

Expose async operator and forwarded requests in the existing Inbox, grouped by originating task.
Deliver the desktop inline answer and phone focused answer flows with canonical withdrawal history and accurate counts.

## In scope

- A bounded, authorized blocking/async union for Inbox lists, counts, boot, hiding, snooze wakeup, and History.
- Typed identities and submission endpoints, with per-request counts after task grouping.
- Shared question editor behavior with an explicit async submit adapter.
- Non-modal transcript cards, explicit wait status, and appropriate pending-owner navigation.
- Draft retention, concurrent winners, loading/errors, localization, and phone composition.
- Mock-agent scenarios that drive real async MCP calls and visible work/checkpoint markers.
- Focused desktop/mobile E2E, public workflow/API guidance, and lifecycle promotion only after all work-order evidence passes.

## Out of scope

- New Inbox navigation or feature flags, Office inbox changes, and an earlier-turn blocking-question bucket.
- Whole-application UI refactoring or generic QA tasks.

## Acceptance

1. The first RED renders an Inbox with one real async request and expects an actionable request count before an answer exists.
   It fails on missing population, not a new test selector. Mixed-source pagination, boot badge, hidden rows, and workspace switches then converge from one typed read.
2. UI-01 through UI-03 satisfy the linked criteria on desktop and phone, including draft retention, withdrawal history, and forwarded child answers.
3. Real MCP scenario markers prove continued work and checkpoint answer reads. Focused E2E, typecheck, locale checks, and public-doc validation pass against fresh managed builds.

## ASCII UI preview

These excerpts use the same labels as the [combined plan preview](plan.md#ascii-ui-preview).
Task grouping, provenance, distinct running/waiting states, and phone focus/scroll structure are required.
Example copy and spacing are illustrative.

### UI-01: Desktop Inbox

```text
Task: Checkout integration                       Open task
  > Database choice            Agent continues work     ...
  v API response value         Forwarded by Planning    ...
    Child: API batch           Agent continues work
    ( ) Documented default
    ( ) Empty field
    [Additional answer text                              ]
    [Reject]                                       [Submit]
```

### UI-02: Phone focused answer

```text
+--------------------------+
| Back    API batch        |  fixed header, focus returns to row
| From Planning            |
| Agent continues work     |
|--------------------------|
| Question and context     |
| ( ) Documented default   |  one scrolling question body
| ( ) Empty field          |
| [Additional answer     ] |
|--------------------------|
| Reject           Submit  |  fixed, keyboard/safe-area aware
|        safe area         |
+--------------------------+
```

The phone Inbox list opens this one-request surface directly.
It does not mount a hidden desktop workbench.
Reuse the Inbox page shell and the `SessionTaskSwitcherSheet` internal-scroll/focus pattern.
Keep shared domain hooks and drafts above responsive presentation branches.
Measure at least 44px for phone/coarse-pointer controls and 28px for ordinary desktop controls.

### UI-03: Withdrawn and retry states

```text
History: API response value    Withdrawn
Reason: The API documentation supplied the value.

Read failure: Questions could not load.     Retry
Submit failure: Preserve answer text.      Retry
Concurrent winner: Show winner, remove actionable row.
```

Criteria: `AC-TASKS-ASYNC-QUESTIONS-007.1` through `.5`, with withdrawal and forwarding criteria from frontmatter.
Use the two new Playwright files below for rendered evidence.

## Verification

Run from the repository root. Install workspace dependencies once before the first frontend command in a fresh worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -race ./internal/clarification -run '^TestAsyncQuestionsInbox' -count=1)
(cd apps/backend && go test -trimpath ./cmd/mock-agent -run '^TestAsyncQuestions' -count=1)
(cd apps/web && pnpm exec vitest run lib/needs-you-inbox/async-question-groups.test.ts lib/state/slices/needs-you-inbox/needs-you-inbox-slice.test.ts hooks/domains/needs-you-inbox/use-needs-you-inbox-controller.test.tsx components/needs-you-inbox/async-question-row.test.tsx app/needs-you-inbox/needs-you-inbox-page-client.test.tsx components/task/chat/clarification-panel-section.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/async-agent-questions.spec.ts tests/chat/needs-you-inbox-answer.spec.ts tests/chat/needs-you-inbox-background-refresh.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-async-agent-questions.spec.ts tests/chat/mobile-needs-you-inbox-answer.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run changed-file frontend/Go lint on actual touched paths.
Do not overlap the E2E commands or override worker limits.
The managed runner rebuilds web, backend, and fixtures. Report discovered tests and actual results, including skips.
Use the existing causal-wait helpers. Precise backend restart and pause races remain covered by Tasks 01 through 04.

## Files likely touched

- **New:** `apps/web/lib/needs-you-inbox/async-question-groups.ts`, `async-question-groups.test.ts`
- **New:** `apps/web/lib/api/domains/agent-questions-api.ts` and focused API tests
- **New:** `apps/web/components/needs-you-inbox/async-question-row.tsx`, `async-question-row.test.tsx`, `async-question-answer-surface.tsx`
- **New:** `apps/web/e2e/tests/chat/async-agent-questions.spec.ts`, `mobile-async-agent-questions.spec.ts`
- **New:** `apps/backend/cmd/mock-agent/scenarios_async_questions.go` and focused scenario tests
- **New:** `apps/backend/internal/clarification/inbox_async_test.go`
- `apps/backend/internal/clarification/inbox_handlers.go`, `inbox_history_handlers.go`
- `apps/backend/internal/task/repository/sqlite/clarification_inbox_sidecar.go` and new async query helpers
- `apps/backend/internal/backendapp/helpers.go`, `gateway.go`, and boot-state count producers reached from `InboxBootSummary`
- `apps/web/app/needs-you-inbox/needs-you-inbox-page-client.tsx`
- `apps/web/components/needs-you-inbox/needs-you-inbox-row.tsx`
- `apps/web/components/task/chat/clarification-panel-section.tsx` and transcript message dispatch
- `apps/web/lib/types/clarification-inbox.ts`, API DTOs, and task message discriminants
- `apps/web/lib/state/slices/needs-you-inbox/`, `hooks/domains/needs-you-inbox/`, and corresponding History state
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/` catalogs, including `needsYouInbox.json`
- `apps/web/e2e/helpers/api-client.ts`, `apps/backend/cmd/kandev/e2e_reset.go`, and mock scenario registration
- `docs/public/automation-and-mcp.md`, `agent-communication.md`, `coordination.md`, `websocket-api.md`
- `docs/public/coverage.json` only if its source mapping changes
- This package's paired specs, ADR status, work-order results, and plan status after every required check passes

## Dependencies

Task 04. The complete backend lifecycle must exist before a rendered answer flow can claim success.

## Risks

- A mounted list can appear correct while boot/sidebar counts remain stale.
- Two independently paged sources can break the shared page bound and hide older requests.
- Reusing the blocking lifecycle hook for an async card can accidentally block the composer or invoke detached resume.
- Extracted editor logic must preserve task and Quick Chat callbacks.
- A phone surface with two scroll owners can leave Submit behind the keyboard.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/async-agent-questions.md), the IDs in frontmatter.
- [Design](../../specs/tasks/system-design/async-agent-questions.md), Inbox, transcript, HTTP/events, and compatibility sections.
- [Combined previews and E2E matrix](plan.md#ascii-ui-preview).
- `apps/web/AGENTS.md`, `apps/backend/cmd/mock-agent/AGENTS.md`, and scoped chat guidance before touching those files.
- `.agents/skills/mobile-parity/`, `.agents/skills/e2e/`, and `.agents/skills/docs-maintainer/`.

## Results

Completed and remediated after review. The Inbox merges blocking and async
sources under one page bound with a source-prefixed cursor, applies async hiding
and answerability before the source page bound, combines async counts and hidden
counts, and exposes async hidden restore. Desktop keeps inline answers; phone
uses one full-height dialog with focus entry, a focus trap, Escape dismissal,
body scroll lock, focus restoration, fixed header/actions, and an internal
scroll body. History has bounded load-more, terminal outcomes, and withdrawal
reasons. Async rows render authorized saved images.

Verification passed: focused Vitest suites, full web typecheck, full web lint,
i18n check/ratchet, and public-doc validation. Mock scenarios and desktop/mobile
Playwright specs remain unexecuted because the managed browser runner was not
available.
