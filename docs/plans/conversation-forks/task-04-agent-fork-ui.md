---
id: "04-agent-fork-ui"
title: "Deliver the preview and new-agent flow"
status: done
wave: 4
depends_on: ['03-destination-admission']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-FORK-001
  - REQ-TASKS-CONVERSATION-FORK-002
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-004
  - REQ-TASKS-CONVERSATION-FORK-005
  - REQ-TASKS-CONVERSATION-FORK-006
acceptance_criteria:
  - AC-TASKS-CONVERSATION-FORK-001.1
  - AC-TASKS-CONVERSATION-FORK-001.2
  - AC-TASKS-CONVERSATION-FORK-001.3
  - AC-TASKS-CONVERSATION-FORK-001.4
  - AC-TASKS-CONVERSATION-FORK-001.5
  - AC-TASKS-CONVERSATION-FORK-002.1
  - AC-TASKS-CONVERSATION-FORK-002.2
  - AC-TASKS-CONVERSATION-FORK-002.3
  - AC-TASKS-CONVERSATION-FORK-002.4
  - AC-TASKS-CONVERSATION-FORK-002.5
  - AC-TASKS-CONVERSATION-FORK-002.6
  - AC-TASKS-CONVERSATION-FORK-003.1
  - AC-TASKS-CONVERSATION-FORK-003.2
  - AC-TASKS-CONVERSATION-FORK-003.3
  - AC-TASKS-CONVERSATION-FORK-003.4
  - AC-TASKS-CONVERSATION-FORK-003.5
  - AC-TASKS-CONVERSATION-FORK-003.6
  - AC-TASKS-CONVERSATION-FORK-004.2
  - AC-TASKS-CONVERSATION-FORK-004.3
  - AC-TASKS-CONVERSATION-FORK-004.4
  - AC-TASKS-CONVERSATION-FORK-004.6
  - AC-TASKS-CONVERSATION-FORK-004.7
  - AC-TASKS-CONVERSATION-FORK-004.8
  - AC-TASKS-CONVERSATION-FORK-005.2
  - AC-TASKS-CONVERSATION-FORK-006.1
  - AC-TASKS-CONVERSATION-FORK-006.2
  - AC-TASKS-CONVERSATION-FORK-006.3
  - AC-TASKS-CONVERSATION-FORK-006.4
  - AC-TASKS-CONVERSATION-FORK-006.5
system_design:
  - ../../specs/tasks/system-design/conversation-forks.md
---

# Task 04: Deliver the preview and new-agent flow

## Summary

Add the message action, shared context chip, responsive preview, and new-agent destination. Prove visible and delivered context match on desktop and phone.

## In scope

- Enforce AC-004.8: agents share the source execution workspace, new tasks use a separate workspace, and child tasks offer both modes.

- Keep Start available when the estimate exceeds the model window. Do not add threshold warnings or summarization controls.
- Own the typed API client, shared domain hook, source-message eligibility, request generations, and recovery states.
- Own destination picker and the preview range, tool-inclusion switch, and attachment controls. Keep unfinished task destination wiring internal until Task 05.
- Own NewSessionDialog integration and launch request references. Do not reuse task primary-session heuristics for source selection.
- Own provenance display after reload, literal historical mentions, keyboard focus, phone layout, and all introduced translations.
- Add the desktop/mobile agent E2E scenarios from the plan with isolated mock-agent delivery evidence.

## Out of scope

Task and child-task submission wiring, arbitrary transcript editing, automatic summaries, and new provider integrations.

## Acceptance

- A finalized message opens the shared fork flow and creates a new agent with the selected history and new instruction.
- Preview changes, model races, expiry, selected evidence, and attachments preserve user input and match the submitted snapshot.
- Desktop, keyboard, and phone flows meet UI-01 through UI-04, including full preview access, target sizes, scrolling, and focus return.

## ASCII UI preview

Use [the combined preview](plan.md#ascii-ui-preview) for UI-01 through UI-04.
The excerpt preserves the same structural contract and the acceptance references in this work order.

```text
UI-02 desktop
[Conversation: messages | estimated tokens | Preview | x]
[New instruction                                      ]
[Profile] [Model] [Executor]             [Primary action]

UI-02 phone, full-height surface
[Back] Destination                         fixed
[Conversation | View | x]
[New instruction]                          scroll body
[Profile >] [Model >] [Executor >]
[Primary action]                           fixed + safe area

UI-03 phone: View replaces the creation body
[Back] Conversation preview                fixed
[Range >] [Section v]
[Complete historical content]              scroll body
[Apply selection]                          fixed + safe area

UI-04
[Expired snapshot] [Rebuild] [x]            input preserved
```

Workspace row on desktop and phone: new agent = Shared, new task = Separate.
Child-task forms offer [Share parent workspace | Separate workspace].
Use tap targets of at least 44 pixels on phone and coarse pointers.
Use one active vertical scroll owner. Back returns to creation without losing input.
Compare rendered desktop and phone surfaces against these labels during the assigned E2E checks.

## Verification

Run from the repository root. Add failing behavioral tests before production changes.
All new test names and files are specified in the plan's coverage table.
A missing test file or selector alone is not behavioral RED evidence.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/task/use-conversation-fork.test.ts components/task/conversation-fork-flow.test.tsx components/task/new-session-form-actions.test.ts components/task/new-session-dialog.test.tsx components/task/chat/messages/message-actions.test.tsx lib/api/domains/conversation-fork-api.test.ts lib/services/session-launch-helpers.test.ts lib/services/session-launch-service.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/conversation-fork-agent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-conversation-fork-agent.spec.ts)
```

The managed E2E runner builds current backend and web artifacts. Run these commands sequentially, with no worker overrides.
Capture the rendered preview during these runs and record any structural mismatch.
Run targeted ESLint on the TS/TSX files changed by this work order before completion.

## Files likely touched

- `apps/web/lib/api/domains/conversation-fork-api.ts` (new)
- `apps/web/hooks/domains/task/use-conversation-fork.ts` (new)
- `apps/web/components/task/conversation-fork-flow.tsx`, `conversation-fork-chip.tsx`, `conversation-fork-preview.tsx` (new)
- `apps/web/components/task/chat/messages/message-actions.tsx` and destination first-message rendering
- `apps/web/components/task/new-session-dialog.tsx`, `new-session-dialog-surface.tsx`, `new-session-form-actions.ts`
- `apps/web/lib/services/session-launch-helpers.ts`, `session-launch-service.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/task.json` and generated pseudo resources
- `apps/web/lib/api/domains/conversation-fork-api.test.ts` (new API contract tests)
- New unit tests named in the plan and existing message-action/new-session tests
- `apps/web/e2e/tests/session/{conversation-fork-agent,mobile-conversation-fork-agent}.spec.ts` (new)
- `apps/web/e2e/tests/session/conversation-fork-helpers.ts` (new)
- Isolated E2E fixture/mock-agent support only if needed for delivered-context assertions

## Dependencies

Task 03 must pass before this work starts.

## Risks

Late preview responses can overwrite newer selections. Hover-only affordances and duplicate mobile overlays can make the context inaccessible.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-forks.md) and [system design](../../specs/tasks/system-design/conversation-forks.md).
- `PromptMentionChip`, `useTouchDrawer`, `mobile-picker-sheet.tsx`, `new-session-form-actions.test.ts`, `mobile-new-session-dialog.spec.ts`, and design section Interface.

## Results

The message action, destination picker, snapshot controls, preview, chip, and task-session provenance are implemented. Finalized user and agent messages in ordinary task sessions can start a new agent with the selected frozen history. Tool evidence remains optional and attachments are copied explicitly. The provenance action sits below the fixed phone top bar so it remains touch-accessible.

Validation passed:

- Focused frontend tests: 141 tests passed across 14 files, including fork API, hook, flow, new-session, message action, launch service, and task destinations.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Managed desktop E2E: all 3 agent and task destination tests passed.
- Managed mobile E2E: all 3 agent and task destination tests passed. The preview, new-task form, and child-task form fit their mobile surfaces with safe-area clearance and touch-sized controls.
- Targeted ESLint completed with 0 errors and warnings after extracting the dialog surface into its own component.

`pnpm run i18n:zh-hant` passed after the existing `workflows:openAgentSettings` phrase was added to the reviewed converter overrides. The fork entries pass the six-catalog completeness check and new-code ratchet.

### Review remediation

The shared hook now keeps the current usable snapshot until a replacement draft and preview both load, ignores stale responses, and accepts a persisted user cutoff during an active assistant turn. A failed replacement leaves the original snapshot available for launch.

Validation passed: `pnpm exec vitest run hooks/domains/task/use-conversation-fork.test.ts` (9 tests), including failed replacement, overlapping requests, stale selection, and active-turn user-cutoff regressions.

### PR review remediation

The flow mounts only while open. Removing the conversation chip keeps the selected destination form open, and Escape or Back from preview returns to creation before dismissing the dialog. On phone, opening the session dialog focuses its title without opening the keyboard. Mobile E2E geometry assertions wait for the drawer animation to settle instead of reading a transient position.

Validation passed: targeted desktop and phone fork-agent E2E with form and preview screenshots; mobile session-dialog and saved-prompt launch regressions passed with retries disabled. Targeted ESLint completed with 0 errors and warnings.
