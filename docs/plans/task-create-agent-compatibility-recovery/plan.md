---
created: 2026-09-04
status: implemented
requirements:
  - REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001
system_design:
  - ../../specs/tasks/system-design/task-create-agent-executor-compatibility.md
legacy_specs: []
---

# Implementation Plan: Task Create Agent Compatibility Recovery

## Overview

Fix [issue #3387](https://github.com/kdlbs/kandev/issues/3387): the task-create
dialog shows "No compatible agent profiles for X" and hides the agent selector
whenever the selected agent is incompatible with the executor, even when other
agents are compatible. The result is a dialog that replaces an incompatible
selection automatically, keeps the selector visible whenever a compatible
profile exists, and names the workflow, agent, and executor when a workflow
lock prevents recovery.

Order: derive the state and the automatic replacement first (pure logic with
unit tests), then the presentation and copy that depend on that state, then the
end-to-end evidence. Each step is independently verifiable.

## Confirmed root cause

- `useExecutorProfileCompat` in `apps/web/components/task-create-dialog-computed.ts`
  folds two conditions into one boolean, `noCompatibleAgent`: the compatible
  list is empty, or the selected agent is absent from it.
- `AgentColumn` in `apps/web/components/task-create-dialog-form-body.tsx`
  renders the same "No compatible agent profiles" empty state for both
  conditions and drops the selector, so the second condition has no recovery
  path in the UI.
- `handleExecutorProfileChange` in
  `apps/web/components/task-create-dialog-handlers.ts` only stores the executor
  id, and `decideAgentProfileAutopick` in
  `apps/web/components/task-create-dialog-autopick.ts` skips with
  `already-set` whenever an agent id is present, so nothing re-validates the
  agent after an executor switch.
- The compatibility rule in `apps/web/lib/agent-executor-compat.ts` is correct.
  The user's Sprites profile had Claude configured, but the pre-selected agent
  was a different agent type.

Reproduction without a browser: the existing
`useDefaultSelectionsEffect` tests in
`apps/web/components/task-create-dialog-effects.test.ts` only cover an empty
`agentProfileId`. A new case with `agentProfileId` set to an incompatible
profile and a non-empty compatible list fails before the fix because
`setAgentProfileId` is never called.

## Scope

### In scope

- A three-way agent compatibility state derived in the dialog's computed hook.
- Automatic replacement of an incompatible, non-locked selection using the
  existing preference order.
- Selector kept visible whenever a compatible profile exists.
- A workflow-locked message naming workflow, agent, and executor.
- Footer disabled reasons that match the shown state.
- Translated copy in all five locales plus the regenerated pseudo locale.
- Desktop and mobile Playwright evidence for the user-visible states.

### Out of scope

- Marking incompatible executors in the executor selector.
- Changing the credential compatibility rule.
- The additional-session, handoff, and quick-chat dialogs.
- Persisting the automatic replacement as the last-used agent.

## Technical approach

### Compatibility state and replacement (`task-create-dialog-computed.ts`, `task-create-dialog-autopick.ts`, `task-create-dialog-types.ts`)

- Add `AgentCompatState` and the pure `computeAgentCompatState` helper next to
  `filterCompatibleAgentProfiles`. `useExecutorProfileCompat` returns
  `agentCompatState`, keeps `noCompatibleAgent` as
  `agentCompatState !== "compatible"`, and returns the effective agent
  profile's label as `selectedAgentProfileName`.
- Extend `AutopickDecision`'s `pick` variant with `replaces?: string`. In
  `getAgentAutopickGate`, evaluate the replacement gate before the `already-set`
  skip. `decideAgentProfileAutopick` reuses the last-used, workspace-default,
  first-compatible candidate chain for the replacement.
- `buildAgentAutopickDebugFields` logs `replaces`.
- `DialogComputedValues` gains `agentCompatState` and
  `selectedAgentProfileName`.

### Presentation and copy (`task-create-dialog-form-body.tsx`, `task-create-dialog-prop-builders.ts`, `task-create-dialog-footer.tsx`, locales)

- `AgentColumn` branches on `agentCompatState` and `workflowAgentLocked` per
  the presentation table in the system design. The new note carries
  `data-testid="agent-profile-incompatible-note"` and reuses the credentials
  link markup from `NoCompatibleAgentState`.
- `buildFormBodyProps` forwards `agentCompatState`, `selectedAgentProfileName`,
  and `effectiveWorkflowName` (looked up from `setup.workflows` by
  `computed.effectiveWorkflowId`).
- `computeDisabledReason` returns `REASON_SELECTED_AGENT_INCOMPATIBLE` for the
  `selected-incompatible` state; `resolveDisabledReason` accepts the agent
  name.
- Add `agentNotConfiguredOnExecutor`, `workflowAgentNotConfiguredOnExecutor`,
  and `selectedAgentNotConfiguredFor` to `task.json` in `en`, `pt-pt`, `zh-cn`,
  then run `pnpm run i18n:zh-hant` for `zh-hk` and `zh-tw` and
  `pnpm run i18n:pseudo` for `pseudo`.

### End-to-end evidence (`apps/web/e2e/tests/task/`)

- The E2E backend registers the mock agent as its only agent type, so every
  profile shares one compatibility result until the backend is restarted with
  `KANDEV_MOCK_PROVIDERS: "codex-acp"`, which registers a mocked second agent
  type (the pattern `executor-agent-config.spec.ts` already uses). A shared
  helper, `agent-compatibility-helpers.ts`, performs that setup: a Codex-alias
  profile, a Docker executor profile without credentials, and a
  `/api/v1/remote-credentials` route mock that requires an env secret for the
  seeded agent and nothing for the alias.
- Desktop: extend `create-task.spec.ts` with two cases: the seeded agent is
  picked on the default executor and replaced by the Codex profile after
  switching to Docker; and a workflow that pins the seeded profile shows the
  workflow-locked note on Docker.
- Mobile: add `mobile-create-task-agent-compatibility.spec.ts` for the
  workflow-locked flow at a phone viewport, asserting the note, the link, the
  disabled start action, and no horizontal document overflow.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| 001.1, 001.6 | `task-create-dialog-computed.test.ts`: `computeAgentCompatState` returns `selected-incompatible`, not `none-compatible`, when a compatible profile exists. `task-create-dialog-form-body.test.tsx`: `CreateEditSelectors` renders the selector and the note, never the empty state, in that state. |
| 001.2, 001.8 | `task-create-dialog-effects.test.ts`: `decideAgentProfileAutopick` returns a pick with `replaces` for an incompatible non-locked selection and honors last-used, default, and first order; `useDefaultSelectionsEffect` calls `setAgentProfileId` with the replacement and never calls `syncTaskCreateLastUsed`. |
| 001.3 | `task-create-dialog-effects.test.ts`: `decideAgentProfileAutopick` skips with `already-set` when the selection is compatible. |
| 001.4 | Existing `CreateEditSelectors` empty-state test in `task-create-dialog-form-body.test.tsx` and the existing Docker E2E case. |
| 001.5 | `task-create-dialog-form-body.test.tsx`: workflow-locked note names workflow, agent, and executor, with the credentials link. `task-create-dialog-effects.test.ts`: no replacement when the workflow locks the agent. |
| 001.7 | `task-create-dialog-footer.test.ts`: `computeDisabledReason` and `resolveDisabledReason` for both states. |
| 001.9 | Mobile Playwright spec below. |

## E2E tests

| Flow | AC | File and project |
| --- | --- | --- |
| Docker executor without credentials shows the empty state and disables start (existing). | 001.4 | `tests/task/create-task.spec.ts`, `chromium` |
| Seeded agent picked on the default executor is replaced by the compatible Codex profile after switching to Docker; no empty state or note; start enabled. | 001.1, 001.2, 001.6 | `tests/task/create-task.spec.ts`, `chromium` |
| Workflow-locked agent on a Docker executor without credentials shows the workflow note, keeps the link, disables start. | 001.5, 001.6, 001.7 | `tests/task/create-task.spec.ts`, `chromium` |
| Same workflow-locked flow on a phone viewport, note wraps, no horizontal overflow. | 001.9 | `tests/task/mobile-create-task-agent-compatibility.spec.ts`, `mobile-chrome` |

## Work orders

- [x] [Task 01: Derive compatibility state and replace incompatible selections](task-01-compat-state-and-replacement.md)
- [x] [Task 02: Present compatibility states with translated copy](task-02-agent-column-presentation.md)
- [x] [Task 03: End-to-end evidence on desktop and mobile](task-03-e2e-evidence.md)

## Dependency order

```text
Task 01 -> Task 02 -> Task 03
```

Task 02 consumes the state contract from Task 01. Task 03 exercises the
rendered result of Task 02 against the production build.

## Verification results

- Task 01: `pnpm exec vitest run` on the computed, effects, workflow-agent
  effect, and effects-executor files: 4 files, 66 tests passed. Typecheck and
  eslint clean.
- Task 02: `pnpm exec vitest run` on form-body, footer, prop-builders plus the
  Task 01 files, dialog, and setup tests: 7 files, 116 tests passed.
  `pnpm run i18n:check`, `pnpm run typecheck`, and eslint on every changed
  production and test file: clean.
- Task 03: `pnpm e2e:raw tests/task/create-task.spec.ts`: 17 passed.
  `pnpm e2e:raw --project=mobile-chrome
  tests/task/mobile-create-task-agent-compatibility.spec.ts`: 1 passed.
- Package gates: `pnpm run i18n:ratchet`, `node scripts/check-new-e2e-sleeps.mjs`,
  `check-no-em-dash-ui.mjs`, prettier, `python3.13 scripts/lint-spec-files.py --all`,
  and `python3.13 scripts/lint-architecture.py --all`: clean.

## Risks

- The replacement effect must not fight the workflow lock effect. The gate
  checks both `workflowAgentProfileId` and the synchronous
  `workflowHasAgent` lookup, matching the existing autopick guard.
- A profile-list refresh could momentarily drop the selected profile. The gate
  requires a non-empty compatible list, so an empty refresh cannot clear a
  selection; a non-empty list that lacks the selection is a real incompatibility
  or a deleted profile, and replacing it is correct.
- The `selected-incompatible` unlocked state is visible for one render before
  the replacement applies. The note is honest for that render and is the only
  recovery path if the effect cannot run.
- The `AgentColumn` function is near the 100-line lint cap once it branches on
  the new state. Extract the note into its own component.
- Copy changes gate the build: `check-i18n-keys.mjs` fails on a missing key or
  an untranslated value in any locale.
