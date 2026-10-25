---
id: "02-create-dialog"
title: "Add task-create controls and lifecycle coverage"
status: done
wave: 2
depends_on: ["01-runtime-contract"]
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.1
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.2
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.3
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.4
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.5
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.6
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.7
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.8
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.9
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.10
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.11
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.12
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
---

# Task 02: Add task-create controls and lifecycle coverage

## Summary

Expose grouped replacements inside Advanced settings. Prove the complete task
lifecycle on desktop and phone, then document the feature.

## In scope

- Extend shared form state, types, payload construction, and retry paths.
- Derive distinct source rows and affected steps, including earlier-step consumers.
- Add responsive profile selectors, reset, loading/error states, and effective summaries.
- Preserve choices on collapse/error and clear them on workflow/workspace/new-task changes.
- Add all five locale catalogs, public how-to text, and related routing design updates.
- Add desktop/mobile mock-agent E2E with actual profile and session assertions.
- Verify effective model/profile previews from both the topbar and above-chat controls, including phone tap equivalents and cross-task cache isolation.

## Out of scope

Existing-task override editing, new profile settings, and workflow-template edits.

## Acceptance

1. One Luna row replaces all matching fixed references for this task. Initial Agent retains its current behavior.
2. Desktop and phone meet UI-01 through UI-03, including loading, errors, reset, and fresh-task isolation.
3. E2E proves Analysis/Review use A and Implement/PR use B with the expected session identities. A second task keeps the workflow default, and a third task keeps its own different override during interleaved execution.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). Apply these same views:

### UI-01: Desktop, create task with Advanced settings expanded

```text
Workflow       [Feature v]
Initial Agent  [Astra Medium v]
[v] Advanced settings
  Depends on [None v]       Priority [Normal v]
  Workflow agents
  Changes apply only to this task.
  5.6 Luna Max             [5.6 Terra High v] [Reset]
  Implement, PR
                                [Create task]
```

### UI-02: Phone, same expanded state

```text
New task                              [Close]
Workflow
[Feature                                  v]
Initial Agent
[Astra Medium                             v]
[v] Advanced settings
Depends on [None                          v]
Priority   [Normal                        v]
Workflow agents
Changes apply only to this task.
5.6 Luna Max
Implement, PR
[5.6 Terra High                           v]
[Reset to workflow profile]
---------------------------------------------
[Create task]
```

The form body scrolls. The existing footer remains reachable above the safe area.
The phone selector opens the existing responsive agent picker. No new screen is
necessary. Source labels and replacement choices are structural requirements.
Spacing and the example replacement are illustrative. These excerpts omit
unchanged task fields and retain their current order.

### UI-03: Loading and errors within Workflow agents

```text
Workflow agents
[Loading workflow agents...]

Workflow agents
Could not load workflow agents. [Retry]

5.6 Luna Max             [Unavailable profile v] [Reset]
Choose an available agent profile.
```

Submission is disabled during unresolved workflow loading/errors and invalid
selections. A successful empty result removes this region. Collapsing Advanced
settings does not discard choices. Criteria .1, .2, .6, .7, .8, and .10 apply.


### UI-04: Existing step disclosure, task-specific model

Entry points: hover/focus in the topbar or above chat. On phones, tap opens the
existing touch disclosure. Keep the current surface composition and controls.

```text
Analysis      Current step
  Agent running  Codex / Astra Medium
Implement     [Move here]
  New session / gpt-5.6-terra
Review        [Move here]
  Reuse initial session / gpt-6-astra
PR            [Move here]
  Reuse Implement session / gpt-5.6-terra
```

This example assumes Terra replaces Luna and the Implement binding exists.
Actual lifecycle labels follow the runtime projection. Before Implement starts,
PR shows `Planned model / gpt-5.6-terra`. Once its session exists, PR shows the
actual effective model. Indeterminate models remain unknown. Never invent a
session binding.
AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.11 applies to both desktop entry points
and their phone tap disclosures.

## Verification

Use TDD for row derivation, state lifetime, payloads, and E2E. Extend the existing
submission suite so no create route drops overrides. Use the workflow-session-
targeting E2E as the session-identity pattern. Record rendered phone evidence.
New E2E names and projects are specified in the plan. Install dependencies once
before the first frontend command in this worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task-create-dialog-workflow-agent-overrides.test.ts components/task-create-dialog-submit.test.tsx components/task/workflow-move-preview.test.tsx components/task/workflow-stepper.test.tsx hooks/domains/kanban/use-workflow-move-preview.test.ts hooks/domains/kanban/use-workflow-move-preview-revision.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint components/task-create-dialog*.ts components/task-create-dialog*.tsx)
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/task/create-task-workflow-agent-overrides.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/task/mobile-create-task-workflow-agent-overrides.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task-create-dialog-advanced-settings.tsx` and a focused workflow-agent control/helper.
- `apps/web/components/task-create-dialog-state.ts`, `task-create-dialog-types.ts`, and `task-create-dialog.tsx`.
- `apps/web/components/task-create-dialog-helpers.ts`, `task-create-dialog-submit.tsx`, and associated tests.
- `apps/web/components/task-create-dialog-form-body.tsx` and shared workflow summaries.
- `apps/web/lib/types/http.ts` and relevant create API types/projections.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`.
- `apps/web/components/task/workflow-stepper.tsx`, `workflow-step-disclosure.tsx`, and shared move-preview components.
- `apps/web/hooks/domains/kanban/use-workflow-move-preview.ts` and its revision helper/tests.
- New desktop/mobile E2E files named in the plan.
- `docs/public/tasks-and-workflows.md` and related fixed-profile routing summaries.

## Dependencies

Task 01 complete. Use its public create field and persisted routing contract.

## Risks

Reuse existing profile eligibility. Do not silently discard an unavailable
selection or persist overrides as last-used settings. Ignore stale workflow
responses. Do not fork desktop/mobile business logic.

## Parallelism

`sequential`

## Inputs

Full requirement/design pair, current Advanced settings, and shared agent picker.
Read `/mobile-parity`, `/e2e`, and `/docs-maintainer` before implementation.

## Confirmed interview cases

- After task creation, change Implement's shared fixed profile from Luna to Sol.
  The overridden task still uses Terra. A task without an override uses Sol.
- Before Implement starts, both PR disclosures show Terra as planned. After
  its session exists, both show its actual effective model and session behavior.
- A new workflow step does not inherit an existing task's replacement.

## Results

Implemented grouped Advanced settings with reset, loading/error retry, workflow
and fresh-task clearing, localized responsive controls, task payload wiring, and
planned-to-actual effective model disclosures. Added desktop and phone mock-agent
E2E coverage for task isolation, reused sessions, workflow edits, reloads, and
touch sizing. The focused frontend suite passed 89 tests, desktop passed 2 tests,
mobile passed 1 test, and the production web build passed. Review remediation
shares override validation with keyboard and alternate create handlers, carries
replacement profiles into preview revisions, and adds rendered desktop,
above-chat, and touch planned-to-actual assertions. Six focused frontend suites
passed 129 tests, with full lint and typecheck passing.
