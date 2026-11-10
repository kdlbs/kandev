---
id: "01-enforce-current-turn-selection"
title: "Enforce current-turn clarification selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-SCENARIOS-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.1
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.2
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.4
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.9
  - AC-TASKS-CLARIFICATION-SCENARIOS-001.1
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
---

# Task 01: Enforce current-turn clarification selection

## Summary

Stop answered or rejected current questions from exposing detached older
questions as active input. Correct the shared selectors and prove the same
result in transcript filtering, pending flags, and desktop/phone chat.

## In scope

- Both detached bypasses in `pending-clarification.ts` and their stale comments.
- Replacement of the conflicting older-detached expectation.
- Unit regressions for authoritative projections, turn ownership, and bundle completeness.
- A shared browser fixture for detached A beside terminal B on an open newer turn.
- Desktop and phone assertions before and after reload, preserving historical controls.

## Out of scope

- Backend changes, response transport, database repair, or agent scenarios.
- New history navigation, layout, copy, dismissal semantics, or permission behavior.
- Changes to ordinary late-answer admission or durable-turn ranking.

## Acceptance

1. Every shared selector respects explicit clean projection and exact durable-turn
   authority. Answered/rejected B cannot reactivate A, including sparse newer-turn history.
2. Current detached questions remain answerable with clarification authority.
   Legacy fallback, same-turn bundles, partial arrival, and terminal siblings retain their behavior.
3. Desktop and phone show normal composer state after B resolves. A stays in
   history with its separate late-answer action, including after reload.

## Implementation sequence

1. Mark this work order `in_progress`. Read its inputs and current caller inventory.
2. Add `does not reactivate an older detached bundle after the current bundle
   is answered` in `pending-clarification-scope.test.ts`. Use distinct turn IDs
   and pending IDs, `currentTurnCompleted=false`, and `pendingAction=null`.
3. Run that regression before production edits. Its failure must select A
   instead of the expected empty group. Add the rejected equivalent.
4. Extend the selector matrix. Cover sparse newer-turn history and clean
   projections over detached predecessors. Include null/undefined authority,
   terminal sessions, and missing-status legacy rows.
5. Add positive controls for current detached questions, including a completed
   durable turn in an input-capable session. Preserve mixed pending/terminal
   siblings, partial-arrival gating, and another eligible same-turn bundle.
6. Remove both detached ownership exceptions. Remove the unused override and
   helper after confirming that production callers do not use them.
7. Add transcript-history and task/session pending-flag coverage. Preserve the
   existing late-answer hook and its message-admission tests.
8. Add the rendered matrix described below. Run the exact final commands and
   record actual counts. Mark the work order `done` only after its checks pass.

## Browser fixture and assertions

Extend `e2e/helpers/clarification.ts` for reuse by desktop and phone. Use the
existing E2E API to create a disposable task and a `RUNNING` session. Seed A
on an older completed turn, B on a newer open turn, and subsequent activity
on B's turn. Give both questions valid metadata and distinct bundle identities.
Parameterize B's persisted status as `answered` or `rejected`.

Use deterministic `createdAt`, `turnStartedAt`, and `turnCompletedAt` values.
Verify the newer turn remains open and the session projection is null. Open
the task directly, assert the UI, reload, and repeat. This exercises real
hydration and selection without requiring a live waiter or response mock.

Required assertions are no active clarification panel, no pending-question
composer wording, and A retained in the transcript. Open and close A's
historical answer surface with the existing controls. It must remain separate
from the active overlay and must not call the tool-response endpoint.
Use `.tap()` on phone. Preserve existing late-message sending tests.

The standard mock agent finishes quickly after an answer. A test that waits
until the newer turn completes does not prove this regression. Keep the
primary RED gate at the selector boundary and record the browser fixture's scope.

## ASCII UI preview

UI-01: Task chat after B resolves, shared affected desktop/phone region.

```text
Transcript: A, B resolved
A: [Answer as new message]
No active clarification panel
Composer: ordinary running-session mode
```

UI-02: A remains current after timeout and projection is `clarification`.

```text
Current detached question A
Answer choices / custom answer
Existing answer and Skip actions
```

The [full plan preview](plan.md#ascii-ui-preview) owns the before/after view
and mobile constraints. Retain the focused phone chat, transcript scroll
owner, inline composer, focus behavior, safe areas, and existing touch targets.
No layout or copy changes are required. UI-01 covers lifecycle AC `.2`, `.4`,
`.9` and scenarios AC `.1`. UI-02 covers lifecycle AC `.1` and scenarios AC `.1`.

## Verification

From the repository root, install workspace dependencies once in a fresh worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

The planning checkout already had dependencies. For RED, run the newly added
regression before production edits:

```bash
(cd apps/web && pnpm exec vitest run lib/utils/pending-clarification-scope.test.ts -t 'does not reactivate an older detached bundle after the current bundle is answered')
```

After implementation, run these commands sequentially from repository root:

```bash
(cd apps/web && pnpm exec vitest run lib/utils/pending-clarification.test.ts lib/utils/pending-clarification-scope.test.ts lib/utils/pending-clarification-permission.test.ts hooks/use-processed-messages.test.ts hooks/use-processed-messages.clarification.test.ts hooks/use-task-pending-input.test.tsx hooks/use-late-clarification-message.test.tsx)
(cd apps/web && pnpm exec eslint lib/utils/pending-clarification.ts lib/utils/pending-clarification.test.ts lib/utils/pending-clarification-scope.test.ts lib/utils/pending-clarification-permission.test.ts hooks/use-processed-messages.test.ts hooks/use-processed-messages.clarification.test.ts hooks/use-task-pending-input.test.tsx e2e/helpers/clarification.ts e2e/tests/chat/clarification-lifecycle-turn-shadow.spec.ts e2e/tests/chat/mobile-clarification.spec.ts)
(cd apps/web && pnpm exec prettier --check lib/utils/pending-clarification.ts lib/utils/pending-clarification.test.ts lib/utils/pending-clarification-scope.test.ts lib/utils/pending-clarification-permission.test.ts hooks/use-processed-messages.test.ts hooks/use-processed-messages.clarification.test.ts hooks/use-task-pending-input.test.tsx e2e/helpers/clarification.ts e2e/tests/chat/clarification-lifecycle-turn-shadow.spec.ts e2e/tests/chat/mobile-clarification.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification-lifecycle-turn-shadow.spec.ts tests/chat/clarification.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Managed E2E rebuilds production assets. Keep desktop and mobile runs separate
and retain the runner's worker limits. Record discovered and executed tests.
Run the copyable Node preflight in the [plan verification section](plan.md#verification)
before marking results complete.

## Files likely touched

- `apps/web/lib/utils/pending-clarification.ts`
- `apps/web/lib/utils/pending-clarification.test.ts`
- `apps/web/lib/utils/pending-clarification-scope.test.ts`
- `apps/web/lib/utils/pending-clarification-permission.test.ts`
- `apps/web/hooks/use-processed-messages.test.ts`
- `apps/web/hooks/use-processed-messages.clarification.test.ts`
- `apps/web/hooks/use-task-pending-input.test.tsx`
- `apps/web/e2e/helpers/clarification.ts`
- `apps/web/e2e/tests/chat/clarification-lifecycle-turn-shadow.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`
- `docs/plans/superseded-clarification-selection/plan.md`
- `docs/plans/superseded-clarification-selection/task-01-enforce-current-turn-selection.md`

## Dependencies

None. Implementation begins after a later explicit user request.

## Risks

- A detached flag changes delivery mode, not turn ownership.
- Explicit clean projection can reflect authority hidden from client turn history.
- Terminal session state differs from completion of the current durable turn.
- Browser seeding must preserve an open newer turn or the defect can be masked.

## Parallelism

`sequential`

## Inputs

- [Lifecycle requirement](../../specs/tasks/requirements/clarification-active-lifecycle.md),
  AC `.1`, `.2`, `.4`, and `.9`.
- [Lifecycle scenarios](../../specs/tasks/requirements/clarification-active-lifecycle-scenarios.md),
  AC `.1`, especially supersession, clean projection, and newer-message deletion.
- [Frontend selection design](../../specs/tasks/system-design/clarification-active-lifecycle.md#frontend-clarification-selection).
- [Current-turn ADR](../../decisions/2026-08-14-current-turn-clarification-ownership.md)
  and [late-answer ADR](../../decisions/2026-09-18-late-clarification-messages.md).
- [Frontend guide](../../../apps/web/AGENTS.md), the chat-scoped guide,
  `/tdd`, `/e2e`, and `/mobile-parity`.
- Existing selector, filtering, pending-input, and late-answer tests.
- Existing lifecycle-turn-shadow seeding and desktop/phone clarification tests.

## Results

Implementation completed on 2026-11-02.

- RED gate: the new answered regression selected `old-question` before the
  production edit, matching the reported defect.
- `pending-clarification.ts` now rejects an explicit non-clarification
  projection before turn filtering, scopes string turn authority exactly,
  suppresses null turn authority, and keeps the projection-gated legacy
  fallback. Detached metadata no longer widens ownership.
- `pending-clarification-scope.test.ts` covers answered and rejected
  transitions, sparse newer-turn history, current detached controls, completed
  turns, null/undefined authority, missing status, and same-turn bundle
  retention.
- Transcript and task/session pending-flag regressions keep superseded A visible
  while active selectors stay empty.
- The seeded desktop and phone matrices pass for answered and rejected outcomes
  before and after reload, including opening and closing A's historical answer
  surface without a tool-response call.
- Focused suites: 7 files, 132 tests. Broader `lib/utils` and `hooks` suites:
  585 files, 5,695 tests. Desktop E2E: 32 tests. Phone E2E: 15 tests.
- Typecheck, ESLint, Prettier, and `i18n:ratchet` passed. Catalog validation,
  36 specification-linter tests, and full specification lint passed.
- Implementation is committed in this PR.
