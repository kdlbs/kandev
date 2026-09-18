---
id: "02-remove-parking-presentation"
title: "Remove parking presentation and verify user flows"
status: pending
wave: 2
depends_on: ["01-repair-session-open-eligibility"]
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.9
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 02: Remove parking presentation and verify user flows

## Summary

Remove the parked-session note from shared chat surfaces. Verify selected
conversation recovery on desktop and phone without adding another parking affordance.

## In scope

- Remove `ParkedSessionNote`, `hasWorkflowParkingMarker`, and their chat-panel use.
- Remove `parkedSessionNote` from en, pseudo, pt-pt, zh-cn, zh-hk, and zh-tw catalogs.
- Replace parked-note tests with absence and ordinary-conversation-control assertions.
- Update desktop/mobile queued-session E2E scenarios according to the plan matrix.
- Verify restart, capacity, prevention preference, conversation identity, and no prompt replay.
- Update both public recovery explanations and affected companion-plan references.

## Out of scope

No replacement parking toolbar, badge, tooltip, or composer restriction. Genuine
queue status remains. No new layout, navigation hierarchy, or automatic prompt.

## Acceptance

1. No parking presentation appears on desktop or phone, including with persisted legacy markers.
2. Opening the selected stopped session resumes it when normal controls permit,
   without clicking Resume. Queue ownership and workflow primary remain unchanged.
3. Both E2E projects and targeted UI/i18n checks pass. Public docs describe the new behavior.

## ASCII UI preview

Use [UI-01 and UI-02](plan.md#ascii-ui-preview).

```text
Desktop: [Session tabs] -> Conversation -> Composer
Phone:   [Session picker] -> Conversation -> Composer -> Bottom navigation
Both:    No parked-session row; genuine task queue status remains when needed.
```

Keep the existing desktop tabs and phone task drawer/session picker.
The chat remains the single conversation scroll owner. No new touch targets or
hover-only behavior are introduced. Cover AC 001.9 and 003.10.

## Verification

Run from repository root; install dependencies once for a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/launch-queue-status.test.tsx hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-queued-session-ownership.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run E2E projects sequentially with the managed runner. Use only disposable fixture
backends and scoped settings. Record behavioral RED before the UI removal and
GREEN after it. Backend Task 01 owns recovery RED; these tests prove integration.
Assert provider readiness independently of workspace-only readiness.

## Files likely touched

- `apps/web/components/task/launch-queue-status.tsx` and its test.
- `apps/web/components/task/task-chat-panel.tsx`.
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`.
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`.
- `apps/web/e2e/tests/workflow/queued-session-ownership.spec.ts`.
- `apps/web/e2e/tests/workflow/mobile-queued-session-ownership.spec.ts`.
- `apps/web/e2e/tests/workflow/queued-session-ownership-helpers.ts`.
- `docs/public/tasks-and-workflows.md` and `docs/public/agents-and-profiles.md`.
- This package's status/results and companion references.

## Dependencies

Task 01. It supplies the backend behavior required by the rendered tests.

## Risks

A hidden note is insufficient if recovery still fails. Do not remove the genuine
queue region or confuse workspace restoration with a resumed provider session.
Restore fixture capacity and user settings even when assertions fail.

## Parallelism

`sequential`

## Inputs

- [Plan, previews, and E2E matrix](plan.md).
- [Design](../../specs/tasks/system-design/queued-session-ownership.md#conversation-recovery-and-workflow-stop-history).
- Existing phone queued-session E2E fixture and session page object.
- Mobile parity and E2E skills; public docs maintenance guidance.

## Results

Pending. No frontend, E2E, or implementation check ran during planning.
