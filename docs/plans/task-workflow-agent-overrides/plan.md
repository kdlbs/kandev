---
created: 2026-09-19
status: done
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
legacy_specs: []
---

# Task workflow agent overrides plan

## Overview

Add task-specific replacements for fixed workflow profiles. Deliver persistence,
validation, and routing first. Then expose the contract in Advanced settings.
Both work orders are complete, including desktop and phone lifecycle evidence.

## Scope

One replacement per distinct fixed step profile, task-only persistence, explicit
session binding compatibility, and desktop/mobile task creation are in scope.
Existing-task editing, workflow mutation, Office/review-action overrides, MCP
schema expansion, and child inheritance are out of scope.

## Sources and technical approach

- [Requirements](../../specs/tasks/requirements/workflow-agent-overrides.md)
- [System design](../../specs/tasks/system-design/workflow-agent-overrides.md)
- Existing `resolveStepAgentProfile` and task service creation own runtime integration.
- A nullable task column stores a typed workflow ID and step-to-replacement bindings, expanded from grouped source-profile choices.
- `TaskCreateAdvancedSettings`, form state, and `buildCreateTaskPayload` own presentation and submission.
- The existing profile-session lifecycle remains authoritative for initial and earlier-step targets.

No companion package currently defines this override capability. Related
profile-session work supplies the existing routing contract, not an additional
implementation dependency.

## ASCII UI preview

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

## Tests

| Criteria | Planned evidence |
| --- | --- |
| .1, .2, .6, .8, .10 | `task-create-dialog-workflow-agent-overrides.test.ts`: grouping, selector reset, stale data, and form lifetime |
| .3, .9 | `workflow_agent_overrides_test.go` in task repository: migration, round trip, unrelated updates, workflow scope |
| .2, .5, .9, .10 | Task service/handler tests: normalization, eligibility, invalid sources, create-only/start, replay |
| .3, .4, .5, .9 | Orchestrator tests: effective routing across initial/ensure/manual/automatic/preflight paths and restart |
| .10 | `task-create-dialog-submit.test.tsx`: all ordinary create payloads and retry retention |

## E2E tests

Create `e2e/tests/task/create-task-workflow-agent-overrides.spec.ts` for chromium
and `mobile-create-task-workflow-agent-overrides.spec.ts` for mobile-chrome.
Use isolated mock-agent fixtures, not the user's active workspace.

The shared scenario creates Initial Agent A and replacement B. It advances
Analysis -> Implement -> Review -> PR and asserts the profile and session ID
at each step. PR must reuse Implement's B session. A second task without an
override must still use the workflow's original profile. A third task with its
own replacement C must continue using C. Interleave transitions across all three
tasks and repeat resolution after reload to detect shared-state leakage. The workflow and
profile records must remain unchanged. Cover create-only followed by later
start, reset, collapse, workflow change, new-task reset, and unavailable choices.
Mobile coverage adds touch sizing, focus return, viewport containment, and no
horizontal overflow with long profile names. Use causal waits, not timed sleeps.

The desktop E2E must open both the topbar and above-chat disclosures and assert
the effective profile/model for fixed, initial, and earlier-step recipients.
Repeat across the three isolation tasks and after reload. Phone E2E opens both
equivalent tap surfaces. Preview requests must not create or start sessions.
Include loading, failed preview, unknown model, and session-model override cases.
These checks cover criterion .11.

The interview added criteria .11 and .12: verify planned-to-actual preview
updates and shared-workflow edits. Change Implement's default from Luna to Sol
after task creation. The overridden task retains Terra; the default task uses
Sol. New step IDs do not inherit previous bindings. Repository tests verify
these bindings survive reload. Both preview entry points show planned Terra
before Implement starts, then its actual effective session model.

## Work orders

- [x] [01: Persist and apply task overrides](task-01-runtime-contract.md)
- [x] [02: Add task-create controls and lifecycle coverage](task-02-create-dialog.md)

Task 02 depends on Task 01. Execute sequentially. No delegation is authorized.

## Verification

Passed on 2026-09-19:

- `go test ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/orchestrator`
- `pnpm exec vitest run` for the six focused frontend suites: 129 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- Desktop Chromium E2E: 2 tests passed.
- Mobile Chromium E2E: 1 test passed.
- `make -C apps/backend build` and `pnpm --filter @kandev/web build:vite`.
- Public documentation and specification validators, plus `git diff --check`.
- Review remediation: task-read failures now stop routing, preflight, and start
  before side effects; create-boundary executor compatibility rejects invalid
  replacements before persistence; and preview revisions include task-bound
  replacement profiles.
- Review regressions passed: full `internal/orchestrator` package, task
  service/handler/backendapp packages, six frontend suites with 129 tests,
  full web lint and typecheck, and both web/backend builds.

## Risks

- A missed resolver caller can make preview, credential preflight, and launch disagree.
- Profile reuse by ID must not change explicit initial/earlier-step session identity.
- Ordinary task updates must preserve the stored map across SQLite and PostgreSQL.
- Workflow snapshots can load after a selection changes. Ignore stale responses.
- Plugin create transports must not silently drop host create fields. Preserve the
  normal payload contract; report unsupported custom transports rather than claim success.

## Documentation impact

During implementation, update `docs/public/tasks-and-workflows.md` as a how-to
section. Explain Advanced settings, task-only scope, reset, and session reuse.
Reconcile fixed-profile resolution summaries in the related system designs.
No public documentation claims that this draft feature already exists.
