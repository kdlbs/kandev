---
id: "01-preserve-source-session-state"
title: "Preserve source-session state during late-answer refresh"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.6
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.8
system_design:
  - ../../specs/tasks/system-design/clarification-response-reliability.md
---

# Task 01: Preserve source-session state during refresh

## Outcome

A late-answer refresh cannot replace a newer session state before ordinary
message admission. A refresh error keeps the answer available and does not
admit a message.

## In scope

- Refresh the original task session before deriving its input mode.
- Reject a stale HTTP snapshot and protect live updates received during the
  request, including activity with an equal timestamp.
- Preserve answers on refresh failure and allow a later attempt to refresh
  before admission.
- Keep ordinary direct-send, queue, and unavailable-session behavior.

## Exclusions

- New backend endpoints, session state, or queue policy.
- Reopening the historical tool request or changing clarification ownership.

## Traceability

- `REQ-TASKS-CLARIFICATION-LIFECYCLE-001`
- `AC-TASKS-CLARIFICATION-LIFECYCLE-001.6`
- `AC-TASKS-CLARIFICATION-LIFECYCLE-001.8`
- `docs/specs/tasks/system-design/clarification-response-reliability.md`

## Implementation acceptance

- Each late-answer attempt reads the source session before ordinary admission.
- An older response or response whose hydration epoch is stale cannot replace
  newer live session state.
- A failed refresh does not admit the message, and the retained answer can be
  retried with a fresh session read.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/use-late-clarification-message.test.tsx hooks/use-late-clarification-message.refresh.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-late-clarification-message.ts hooks/use-late-clarification-message.test.tsx hooks/use-late-clarification-message.refresh.test.tsx)
(cd apps/web && pnpm exec prettier --check hooks/use-late-clarification-message.ts hooks/use-late-clarification-message.test.tsx hooks/use-late-clarification-message.refresh.test.tsx)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

The implementation and deterministic regression cases are present on the PR
branch. Hosted CI is still running. The work order remains in progress until
the current head's required checks finish.
