---
created: 2026-10-02
status: implemented
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
legacy_specs: []
---

# Implementation plan: Quick Chat opening composer

## Overview

Replace the separate Quick Chat setup form with the approved opening composer.
First make opening-message delivery attachment-aware. Then integrate the shared
composer and draft lifecycle. Finish with browser regression coverage and docs.
Execute sequentially with TDD. Implementation started after the explicit request
on 2026-10-02.

## Inputs and ownership

- [Requirements](../../specs/tasks/requirements/quick-chat-opening-composer.md).
- [System design](../../specs/tasks/system-design/quick-chat-opening-composer.md).
- [Repository isolation ADR](../../decisions/0038-quick-chat-repository-isolation.md).
- [Attachment ADR](../../decisions/2026-08-04-file-backed-prompt-attachments.md).

Tasks owns creation and first-message delivery. UI remains responsible for the
existing dialog, tabs, and conversation viewport. No system README change or new
ADR is needed. The approved preview settles the product choices; no material
question blocks this package.

## Scope

### In scope

The opening composer, agent/default selection, optional repository chips,
configuration toggle, attachment transfer, existing voice integration, failure
recovery, localization, desktop/phone layouts, and targeted regression evidence.

### Out of scope

New voice services, MCP modes, workflow/executor controls, configuration repositories,
cross-device drafts, floating configuration panel redesign, and terminal/tab redesign.

## Technical approach

Task 01 extends `useQuickChatInitialPrompt` and the Quick Chat store/session plumbing
from a string to a full opening payload. It also carries terminal-backed prompt
attachments through the existing backend launch paths. Existing callers keep
working through a string compatibility bridge.

Task 02 owns the visible integration in `quick-chat-setup.tsx`,
`quick-chat-modal.tsx`, and the setup state around `useQuickChatModal`.
Extract shared input pieces from `TaskFormInputs` only where reuse requires it.
Use the existing `ChatInputPluginActions` slot for quick-chat voice controls.
Update quick-chat/config-chat hooks and API types for the payload handoff.
Preserve draft ownership across mode changes, tabs, and viewport changes.

Task 03 updates shared browser helpers and every affected direct setup caller.
It proves one-send behavior, attachment recovery, mobile geometry, and configuration
routing. It also updates public instructions and replaces obsolete setup-only
helper-copy/footer clauses in the repository-context requirement when behavior lands.

### Compatibility matrix

| Consumer              | Transport/identity                                     | Intended behavior                            | Evidence / fallback                                       |
| --------------------- | ------------------------------------------------------ | -------------------------------------------- | --------------------------------------------------------- |
| Structured Quick Chat | Session-bound message handler                          | Send once after subscription/admission       | Hook tests and mock-agent E2E; recover same-session draft |
| Structured config     | Config metadata plus message handler                   | Same composer, config tools                  | Config tests and E2E; preserve eligibility gate           |
| CLI passthrough       | HTTP plus launch Prompt/Attachments                    | One prompt-bearing process start             | Go launch tests; existing explicit start recovery         |
| Voice plugin          | Existing quick-chat capability, null IDs before create | Insert and optional submit into active draft | Fixture capability test; no control when unavailable      |
| Legacy launcher       | Existing optional prompt/string handoff                | Existing programmatic behavior               | Existing config and saved-prompt regressions              |

## ASCII UI preview

Structure and control order are required; spacing and copy are illustrative.
All production copy uses locale keys. UI-01 through UI-04 cover AC 1-12.

### UI-01: Desktop, New Chat, empty draft

```text
+----------------------------------------------------------------+
| Chat 1 x    New Chat x    +                                     |
+----------------------------------------------------------------+
|                                                                |
|              What would you like to discuss?                    |
|                                                                |
|       +------------------------------------------------+       |
|       | Write a prompt...                              |       |
|       |                                                |       |
|       | [Attach]                       [Mic] [Send ^]   |       |
|       +------------------------------------------------+       |
|       [Codex / Astra Medium v]      [+ Add repository]          |
|       [off] Configuration session                              |
|                                                                |
+----------------------------------------------------------------+
```

The tab strip is fixed. The remaining setup region owns scrolling. Center the
composer when height permits. Send is disabled for an empty draft. Mic represents
an installed compatible voice action, not an unconditional host control.

### UI-02: Desktop, expanded selections and configuration

```text
       [kandev / origin/main v  x] [+ Add repository]
       +------------------------------------------------+
       | [screenshot.png x]                             |
       | Explain session routing.                       |
       | [Attach]                       [Mic] [Send ^]   |
       +------------------------------------------------+
       [Codex / Astra Medium v]
       [off] Configuration session

       Configuration mode (same draft, repositories hidden):
       +------------------------------------------------+
       | [screenshot.png x]                             |
       | Explain session routing.                       |
       | [Attach]                       [Mic] [Send ^]   |
       +------------------------------------------------+
       [Codex / Astra Medium v]
       [on] Configuration session
       Allow the agent to update Kandev settings,
       workflows, agent profiles, and MCP configuration.
```

The selected repositories survive mode changes but never enter a config request.
The enabled explanation is inline, not a tooltip. No Cancel/Start footer exists.

### UI-03: Phone, New Chat and picker

```text
+----------------------------------+   +----------------------------------+
| < Chats             New Chat     |   | New Chat draft remains behind    |
+----------------------------------+   |                                  |
| What would you like to discuss?  |   |                                  |
| +------------------------------+ |   | +------------------------------+ |
| | Write a prompt...            | |   | | Choose agent                 | |
| |                              | |   | | [Search agents...]           | |
| | [Attach]    [Mic] [Send ^]    | |   | | Codex / Astra Medium       o | |
| +------------------------------+ |   | | Claude / profile           o | |
| [Codex / Astra Medium v]         |   | |                              | |
| [+ Add repository]              |   | +------------------------------+ |
| [off] Configuration session     |   +----------------------------------+
+----------------------------------+
```

Navigation stays fixed. The form has one vertical scroll owner, safe-area padding,
and no page-level horizontal scroll. Temporary choices use inset bottom pickers.
Phone controls have at least 44px hit areas. Reduce heading space when the keyboard
opens; the editor and Send remain reachable. Desktop spacing must not leak here.

### UI-04: Pending uploads, start, and failure

```text
| [trace.zip: uploading...]                     [Send: disabled] |
| [trace.zip: upload failed] [Retry] [Remove]    [Send: disabled] |
| [trace.zip: ready x]                         [Starting...]    |
| Could not start this chat. Your draft is kept.                |
| [Attach]                                      [Send ^]       |
```

A failure before session allocation remains in setup with its recoverable prompt
and files. A launch failure after allocation opens the retained session for
explicit recovery. Post-creation delivery failure remains in the created session
with its recoverable prompt and files. An explicit retry does not create another
conversation. Busy state disables duplicate submission and mode changes. Use live
status/error announcements without moving keyboard focus.

## Tests

Test paths below define traceability targets. Implementation status and executed
verification are recorded in the Results sections. Use `@covers` annotations for
these AC IDs when adding cases.

| AC suffix     | Test evidence                                                                                                                                                                            |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1, 3, 4, 5, 8 | `components/quick-chat/quick-chat-setup.test.tsx`, `use-quick-chat-setup-draft.test.ts`: immediate editing, defaults/disabled profiles, repo validation, mode round trip, duplicate Send |
| 6, 7          | Setup and task-create selector tests; desktop and mobile `composer-actions.spec.ts`: attachment recovery and plugin insertion/submission                                                 |
| 8, 9, 10      | `use-quick-chat-initial-prompt.test.ts`: full payload, rejected/throwing admission, remount, new draft race, stable message identity                                                     |
| 5, 8, 9       | `use-quick-chat-modal.test.ts`, `components/config-chat/use-config-chat.test.ts`: routing, superseded starts, workspace change, preserved payload                                        |
| 8, 9, 12      | `task_http_quick_chat_opening_test.go`: Quick Chat/config opening payload and attachment rollback; handler/service/repository tests                                                      |
| 9, 10         | `lib/state/slices/ui/quick-chat-actions.test.ts`, `quick-chat-sync.test.ts`: payload retention and reconciliation                                                                        |
| 10, 12        | Draft and recovery tests: descriptor restore, invalid/expired records, identity/workspace scope                                                                                          |

## E2E tests

| File and project                                                             | Flow / AC suffix                                                                                     |
| ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `tests/chat/quick-chat-opening-composer.spec.ts`, chromium (new)             | UI-01/02/04; one initial message, failed creation recovery/retry, configuration routing; AC 1-10, 12 |
| `tests/chat/mobile-quick-chat-opening-composer.spec.ts`, mobile-chrome (new) | UI-03/04; touch pickers, prompt/file/config flow, controls and overflow; AC 1-12                     |
| Desktop and mobile `tests/plugins/*composer-actions.spec.ts`                 | Installed plugin inserts at selection and submits the current Quick Chat opening payload             |
| Existing desktop/mobile Quick Chat and composer specs                        | Entry, repositories, saved prompts, tabs, queueing, entity references, slash commands, and settings  |

Use the installed plugin fixture to prove composer capability insertion and submit.
Real microphone capture is not required by mock E2E; manually check a configured
voice plugin during implementation if available and report that evidence separately.
Browser geometry proves reduced-height behavior; record a real-device keyboard
check if available without claiming desktop emulation proves native keyboard behavior.

## Work orders

- [x] [Task 01: Preserve and deliver the complete opening payload](task-01-opening-payload.md)
- [x] [Task 02: Build the shared desktop and phone composer](task-02-shared-composer.md)
- [x] [Task 03: Prove browser flows and update user documentation](task-03-browser-and-docs.md)

Each work order depends on the previous one. No delegation is authorized.

## Companion packages

The implemented saved-prompt, composer-attachment-scope, preparation-attachment-preview,
and configuration-launcher packages describe retained behavior. Do not reset their
completed statuses. New evidence belongs here. Inventory old setup-helper callers
before implementation; update affected tests rather than weakening their assertions.

## Verification results

- Task 01 backend handler, service, and SQLite repository tests passed with
  `go test ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite`.
- Focused web tests passed: 28 files, 316 tests. `pnpm run typecheck` and focused
  ESLint passed with no warnings. Traditional Chinese, pseudo-locale, i18n check,
  and i18n ratchet commands passed.
- The managed E2E runner built the backend targets and Vite production bundle.
  The desktop and phone opening-composer flows passed, as did creation-failure
  retry, desktop and mobile installed-plugin insertion/submission, and a check
  that one opening prompt is persisted exactly once.
- Existing desktop regression set: 57 tests were covered; 53 passed in the broad
  run and the four migrated setup/queue cases passed in focused reruns. Existing
  mobile Quick Chat/config set: 15 tests covered, with the migrated configuration
  picker case passing in a focused rerun after the other 14 passed. Desktop
  configuration-popover set: six cases covered, including the migrated
  command-palette setup case rerun successfully.
- Public-document validation, documentation-coverage validation, specification
  catalog validation, all specification-linter tests, full spec lint, and
  `git diff --check` passed after the results/status updates.
- Merged-base PR fixup verification passed: backend task-handler tests including
  race detection, orchestrator tests, 23 Quick Chat frontend test files (151
  tests), 15 changed frontend test files (225 tests), the deferred-upload
  lifecycle test, typecheck, changed-file ESLint/Prettier, i18n check/ratchet,
  desktop setup-recovery E2E (4 tests), mobile setup-recovery E2E (2 tests), and
  the public-document and specification validators.

Implementation resolves the identified risks: mode changes preserve the setup
draft, both launch paths carry the opening payload once, attachment rollback
restores recoverable staging ownership, and migrated browser callers now assert
the first-turn behavior. Existing repository isolation, permission, and launch
eligibility checks remain authoritative.

## Review follow-up

The review identified three regressions against existing payload and recovery
contracts. Task 01 covers authenticated ownership in detached configuration
launches and session-bound deferred delivery. Task 02 covers upload completion
across setup unmounts, including rejection and explicit discard. The existing
requirement and system design already define the intended behavior.

All three findings are resolved. The owner and foreign-user upload-claim cases,
the deferred-upload tab-switch/retry/discard cases, and the unblocked
session-generation case pass. Verification after these fixes: Go handler tests;
31 affected frontend test files (320 tests); web typecheck; focused ESLint with
no warnings; Vite production build; and `git diff --check`.
