---
id: "01-compat-state-and-replacement"
title: "Derive compatibility state and replace incompatible selections"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.2
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.3
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.6
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.8
system_design:
  - ../../specs/tasks/system-design/task-create-agent-executor-compatibility.md
---

# Task 01: Derive compatibility state and replace incompatible selections

## Summary

Replace the single `noCompatibleAgent` boolean with a derived compatibility
`AgentCompatState` and teach the agent autopick to replace a selection that
became incompatible after an executor switch. Both changes are pure logic with
unit tests and do not touch rendering.

## In scope

- Add `AgentCompatState` and `computeAgentCompatState` to
  `task-create-dialog-computed.ts`; return `agentCompatState` and
  `selectedAgentProfileName` from `useExecutorProfileCompat` and
  `useDialogComputed`; keep `noCompatibleAgent` derived from the state.
- Keep an unlocked disabled or dynamic-off selection in a distinct
  `selected-unavailable` state while a compatible alternative exists. Preserve
  the `none-compatible` behavior for a locked disabled profile.
- Add the replacement gate to `getAgentAutopickGate` and the `replaces` field
  to the `pick` decision in `task-create-dialog-autopick.ts`; log `replaces`
  in the autopick debug fields.
- Extend `DialogComputedValues` in `task-create-dialog-types.ts`.
- Regression tests listed under Verification.

## Out of scope

- Any change to `AgentColumn`, the footer, prop builders, or locale files.
- Any change to `isAgentConfiguredOnExecutor` or the remote-auth catalog.
- Writing the replacement to the last-used preference.

## Acceptance

- `computeAgentCompatState` returns `selected-incompatible` when the effective
  agent fails the executor credential check, `selected-unavailable` for an
  unlocked disabled or dynamic-off selection when another compatible profile
  exists, `none-compatible` when no profile passes (including a locked disabled
  profile), and `compatible` otherwise.
- With an executor selected, auth loaded, no workflow lock, and a non-empty
  compatible list that lacks the current selection,
  `decideAgentProfileAutopick` returns a `pick` whose `replaces` is the current
  id, chosen in last-used, workspace-default, first-compatible order; with a
  workflow lock or a compatible selection it returns the existing skip.
- `useDefaultSelectionsEffect` applies the replacement through
  `setAgentProfileId` and never calls `syncTaskCreateLastUsed`.

## Verification

Write the failing hook test first: `agentProfileId` set to an incompatible
profile, two compatible profiles, expect `setAgentProfileId` to be called with
the first compatible one. Confirm it fails before the production change, then
run:

```bash
# Fresh worktree only, once:
cd apps && pnpm install --frozen-lockfile
# From apps/web:
pnpm exec vitest run components/task-create-dialog-computed.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-workflow-agent-effect.test.ts components/task-create-dialog-effects-executor.test.ts
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/task-create-dialog-computed.ts components/task-create-dialog-autopick.ts components/task-create-dialog-types.ts
```

## Files likely touched

- `apps/web/components/task-create-dialog-computed.ts`
- `apps/web/components/task-create-dialog-computed.test.ts`
- `apps/web/components/task-create-dialog-autopick.ts`
- `apps/web/components/task-create-dialog-effects.test.ts`
- `apps/web/components/task-create-dialog-types.ts`

## Dependencies

None.

## Risks

- The replacement gate must run before the `already-set` skip but after the
  `closed`, `workflow-locked`, and `workflow-has-agent` skips, or a locked
  selection could be replaced.
- `useWorkflowAgentProfileEffect` also writes `agentProfileId`. Keep the
  replacement gated on the same lock inputs so the two effects never alternate.

## Parallelism

`sequential`

## Inputs

- System design sections "Data and contracts" and "Control flow".
- Existing `decideAgentProfileAutopick` tests and the
  `useDefaultSelectionsEffect` harness in `task-create-dialog-effects.test.ts`.

## Results

- Added `AgentCompatState` (`task-create-dialog-types.ts`) and the pure
  `computeAgentCompatState` helper; `useExecutorProfileCompat` now returns
  `agentCompatState` and `selectedAgentProfileName`, with `noCompatibleAgent`
  derived from the state.
- `decideAgentProfileAutopick` evaluates `selectionNeedsReplacement` before the
  `already-set` skip and returns a `pick` carrying `replaces`; the debug fields
  log it.
- RED: 9 new tests failed (helper missing, decision returned `already-set`,
  hook never called `setAgentProfileId`). GREEN: all pass. One legacy test that
  asserted an incompatible pre-set selection is left alone was removed because
  AC 001.2 supersedes it; the compatible-selection half is covered by the new
  "keeps a selection that stays compatible" test.
- `pnpm exec vitest run` on the four named files: 4 files, 66 tests passed.
  `pnpm run typecheck`: clean. `pnpm exec eslint --max-warnings 0` on the three
  production files: clean.

Follow-up verification added the `selected-unavailable` state for unlocked
disabled or dynamic-off selections with an alternative, plus submit-guard
coverage in `task-create-dialog-setup.test.ts`.
