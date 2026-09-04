---
id: "02-agent-column-presentation"
title: "Present compatibility states with translated copy"
status: done
wave: 2
depends_on: ["01-compat-state-and-replacement"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.1
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.4
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.5
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.6
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.7
system_design:
  - ../../specs/tasks/system-design/task-create-agent-executor-compatibility.md
---

# Task 02: Present compatibility states with translated copy

## Summary

Render the agent column and the footer's disabled reason from
`agentCompatState`. Keep the selector visible whenever a compatible profile
exists, add the incompatible note for the unlocked and workflow-locked cases,
and add the copy in every locale.

## In scope

- `AgentColumn` in `task-create-dialog-form-body.tsx` branches per the
  presentation table; extract an `IncompatibleAgentNote` component carrying
  `data-testid="agent-profile-incompatible-note"` and the credentials link.
- `buildFormBodyProps` in `task-create-dialog-prop-builders.ts` forwards
  `agentCompatState`, `selectedAgentProfileName`, and `effectiveWorkflowName`;
  extend `DialogFormBodyProps` and `CreateEditSelectorsProps`.
- `task-create-dialog-footer.tsx`: add `REASON_SELECTED_AGENT_INCOMPATIBLE`,
  return it for `selected-incompatible`, and resolve it with the agent name.
- Locale keys `agentNotConfiguredOnExecutor`,
  `workflowAgentNotConfiguredOnExecutor`, and
  `selectedAgentNotConfiguredFor` in `en`, `pt-pt`, `zh-cn`; regenerate
  `zh-hk` and `zh-tw` with `pnpm run i18n:zh-hant` and `pseudo` with
  `pnpm run i18n:pseudo`.
- Component and footer tests listed under Verification.

## Out of scope

- State derivation and the autopick (Task 01).
- Playwright coverage (Task 03).
- The new-session dialog's own empty state.

## Acceptance

- In the `selected-incompatible` state without a workflow lock,
  `CreateEditSelectors` renders the agent selector and the note, and does not
  render `agent-profile-empty-state`.
- In the `selected-incompatible` state with a workflow lock, the note names
  the workflow, the agent profile, and the executor profile, links to
  `/settings/executors/<id>`, and no selector is rendered.
- `computeDisabledReason` returns `REASON_SELECTED_AGENT_INCOMPATIBLE` for
  `selected-incompatible` and `REASON_NO_COMPATIBLE_AGENT` for
  `none-compatible`; `pnpm run i18n:check` passes with the new keys.

## Verification

Write the failing component test first (selector plus note in the unlocked
incompatible state). Confirm it fails before the production change, then run:

```bash
# From apps/web:
pnpm exec vitest run components/task-create-dialog-form-body.test.tsx components/task-create-dialog-footer.test.ts components/task-create-dialog-prop-builders.test.ts
pnpm run i18n:check
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/task-create-dialog-form-body.tsx components/task-create-dialog-footer.tsx components/task-create-dialog-prop-builders.ts components/task-create-dialog-types.ts
```

## Files likely touched

- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-form-body.test.tsx`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog-prop-builders.test.ts`
- `apps/web/components/task-create-dialog-footer.tsx`
- `apps/web/components/task-create-dialog-footer.test.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`

## Dependencies

Task 01.

## Risks

- `AgentColumn` is close to the 100-line function cap. Extract the note and
  the state switch into small components rather than growing the function.
- The i18n ratchet rejects any literal string on a changed line. Route every
  new string, including the note's link label, through `t()`.
- Do not use the Unicode em dash in the new copy; the existing footer key uses
  an arrow, which the check allows.

## Parallelism

`sequential`

## Inputs

- System design section "Presentation" and "Mobile composition".
- Existing `NoCompatibleAgentState` markup and the `CreateEditSelectors` test.
- `docs/i18n.md` for the locale workflow.

## Results

- `AgentColumn` branches on `agentCompatState`; the new `IncompatibleAgentNote`
  (`agent-profile-incompatible-note`) renders under the selector in the
  unlocked case and replaces it in the workflow-locked case, naming workflow,
  agent, and executor. The hardcoded "this executor" fallback now resolves
  through `task:thisExecutor`.
- `DialogFormBodyProps`, `CreateEditSelectorsProps`, and
  `CreateModeSelectorsProps` carry `agentCompatState`,
  `selectedAgentProfileName`, and `effectiveWorkflowName`; `buildFormBodyProps`
  resolves the workflow name through the new `resolveWorkflowName` helper.
- Footer: `REASON_SELECTED_AGENT_INCOMPATIBLE` (`task:selectedAgentNotConfiguredFor`)
  for the selected-incompatible state in both start-task and session-default
  paths; `resolveDisabledReason` takes the agent name.
- Locale keys `agentNotConfiguredOnExecutor`,
  `workflowAgentNotConfiguredOnExecutor`, `selectedAgentNotConfiguredFor`, and
  `selectedAgentProfileFallback` added to en, pt-pt, zh-cn; zh-hk and zh-tw
  values taken from `pnpm run i18n:zh-hant` output (the converter also rewrote
  unrelated keys in 21 files, which were reverted); pseudo regenerated.
- RED: 7 tests failed (missing helper, footer key, component states). GREEN:
  `pnpm exec vitest run` on form-body, footer, prop-builders plus the Task 01
  files, dialog and setup tests: 7 files, 116 tests passed. `pnpm run
  i18n:check`: all checks pass. `pnpm run typecheck`: clean. `pnpm exec eslint
  --max-warnings 0` on the six production files and five test files: clean.
