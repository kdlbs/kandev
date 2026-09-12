---
id: "03-transcript-evidence"
title: "Transcript visibility evidence"
status: done
wave: 3
depends_on: ["02-first-message-composition"]
plan: "plan.md"
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
acceptance_criteria:
  - AC-TASKS-INITIAL-TASK-BRIEF-001.1
  - AC-TASKS-INITIAL-TASK-BRIEF-001.2
  - AC-TASKS-INITIAL-TASK-BRIEF-001.3
  - AC-TASKS-INITIAL-TASK-BRIEF-001.7
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
---

# Task 03: Transcript visibility evidence

## Summary

Prove that the stored first prompt replaces the synthetic brief without losing content.
Document the behavior only after the implementation passes its focused checks.

## In scope

- Extend the processed-message regression with a combined stored prompt #1.
- Add desktop and phone composer/reload flows using mock-agent fixtures.
- Add a concise explanation to the existing public task guide.
- Record actual command results in all work orders and the manifest.

## Out of scope

Do not change unrelated launch policy, historical messages, or public API fields.

## Acceptance

- Both desktop and phone preserve brief plus instruction after submission and reload.
- The synthetic row disappears, prompt #1 exists once, and later messages do not repeat the brief.
- Long content remains reachable with existing controls and no document-level horizontal overflow.

## ASCII UI preview

Same UI-01 as [the package preview](plan.md#ascii-ui-preview), covering `AC-TASKS-INITIAL-TASK-BRIEF-001.2` and `.7`.

```text
UI-01: Chat after first message (desktop and phone)

Before                         Proposed
+-------------------------+    +-------------------------+
| User #1                 |    | User #1                 |
| rebase on latest main   |    | Original task brief     |
|                         |    |                         |
| Agent response          |    | rebase on latest main   |
+-------------------------+    | Agent response          |
| Composer         [Send] |    +-------------------------+
+-------------------------+    | Composer         [Send] |
                               +-------------------------+
```

Both viewports retain their existing Chat surface and vertical scroll owner.
Phone controls keep the existing safe-area and touch behavior. No new controls are planned.

## Verification

Start with failing behavior assertions before implementation. Then run this
block from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-processed-messages-fallback.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/initial-task-brief.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-initial-task-brief.spec.ts)
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/use-processed-messages-fallback.test.ts`
- `apps/web/e2e/tests/chat/initial-task-brief.spec.ts (new)`
- `apps/web/e2e/tests/chat/mobile-initial-task-brief.spec.ts (new)`
- `apps/web/e2e/tests/chat/initial-task-brief-helpers.ts (new, if shared)`
- `docs/public/tasks-and-workflows.md`
- `docs/plans/initial-task-brief/plan.md`

## Dependencies

Task 02 must be complete.

## Risks

Use fresh managed builds and causal waits. A seeded API assertion alone is not browser evidence.
Do not weaken existing transcript pagination or fallback eligibility to satisfy these tests.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/initial-task-brief.md).
- [System design](../../specs/tasks/system-design/initial-task-brief.md).
- [Package evidence and test matrix](plan.md).

## Results

- Extended the processed-message regression so a persisted combined prompt replaces the synthetic task-description row without duplicating the brief.
- Added managed desktop and phone Chat flows that submit the first message, reload, verify the stored prompt, send a later message, and check for document overflow.
- Added the same prepared-session API option to both E2E flows without changing production API behavior.
- Added public guidance to `docs/public/tasks-and-workflows.md`.
- `pnpm exec vitest run hooks/use-processed-messages-fallback.test.ts` passed.
- `pnpm e2e:run --project chromium tests/chat/initial-task-brief.spec.ts` passed.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-initial-task-brief.spec.ts` passed.
