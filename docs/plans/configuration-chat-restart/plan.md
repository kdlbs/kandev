---
created: 2026-09-29
status: implemented
requirements:
  - REQ-TASKS-CONFIG-CHAT-RESTART-001
system_design:
  - ../../specs/tasks/system-design/configuration-chat-restart.md
legacy_specs: []
---

# Implementation Plan: Configuration Chat Restart

## Overview

Add an explicit replacement action to the floating Configuration Chat header.
Implement the stop/delete/start backend contract first, then connect the shared
client lifecycle and responsive header. Both work orders were implemented
sequentially with TDD in the primary session after the implementation request.

The [requirements](../../specs/tasks/requirements/configuration-chat-restart.md)
and [system design](../../specs/tasks/system-design/configuration-chat-restart.md)
own behavior and technical boundaries. The existing
[Quick Chat resume package](../quick-chat-session-resume/plan.md) remains complete;
this package adds explicit replacement without reopening its historical work.

## Scope

In scope: the Configuration Chat header, confirmation, strict old-agent stop,
canonical deletion, blank replacement, errors, shared-state reconciliation,
localization, desktop/mobile coverage, and public recovery guidance.

Out of scope: ordinary chats/tasks, rollback of configuration changes, an
expanded-dialog restart button, a second conversation store, global cleanup
changes, and provider-specific restart implementations.

## Technical approach

- New `httpRestartConfigChat` and a narrow orchestrator retirement boundary
  compose existing task authorization, deletion preview, synchronous stop,
  `DeleteTaskWithLifecycle`, and `LaunchSession`.
- The existing quick-chat list projects transient restart admission; competing
  configuration creation is blocked until settlement, including after response
  loss or a browser reload. No persistent operation ledger is introduced.
- Add a typed restart API call; extend `useConfigChat`'s shared admission and
  registration without treating accepted restarts as abandoned setup starts.
- Extract the header and confirmation as needed; keep all state/error behavior
  above the conditional session/setup body.
- Preserve profile and executor selection; map restart errors to localized
  recovery actions. Scope all asynchronous results to their captured identities.

| Runtime shape | Restart behavior | Evidence / fallback |
| --- | --- | --- |
| ACP profile | Blank config-mode task/session, explicit start, no prompt replay | Backend ordering and hook tests; browser sends a new prompt |
| CLI passthrough | Same replacement lifecycle; one explicit terminal launch | Backend/hook tests and passthrough browser case |
| Stopped or failed agent | Inspect runtime ownership; confirmed absence allows replacement | Missing-runtime and stale-runtime backend tests |
| Unavailable profile/executor or uncertain remote stop | Preserve old conversation and show error | Preflight/stop failure tests; no silent provider switch |

## ASCII UI preview

### UI-01: Panel header, persisted conversation

```text
Desktop
+----------------------------------------------------+
| Configuration Chat      [Restart] [Expand] [Close] |
+----------------------------------------------------+
| Existing conversation                             |
|                                                   |
+----------------------------------------------------+

Phone (same panel, touch-sized controls)
+----------------------------------------+
| Configuration...   [R] [Expand] [Close] |
+----------------------------------------+
| Conversation owns the scroll region    |
+----------------------------------------+
```

R is the restart icon with accessible name Restart session, not literal UI
copy. Header remains fixed. Desktop controls measure 28px; phone/coarse-pointer
controls measure at least 44px in both dimensions. Setup shows Restart disabled.

### UI-02: Confirmation and restart feedback

```text
Desktop anchored confirmation       Phone inset bottom confirmation
+------------------------------+   +----------------------------------+
| Restart session?             |   | Restart session?                 |
| Delete this conversation and |   | Delete this conversation and     |
| start a new one.             |   | start a new one.                 |
| [Cancel]   [Restart session] |   | [Cancel]       [Restart session] |
+------------------------------+   +----------------------------------+

Panel during restart              Panel after startup failure
+------------------------------+  +----------------------------------+
| Configuration Chat [R:busy] X |  | Configuration Chat [R] ...       |
| Restarting session...        |  | Previous conversation cleared.   |
|                              |  | New session could not start.     |
|                              |  | [Retry starting session]         |
+------------------------------+  +----------------------------------+
```

Control order, header placement, confirmation consequence, separate status,
and usable phone hitboxes are structural requirements (AC .1, .2, .6-.9).
Spacing and prose are illustrative; final copy is localized. Close stays
available during accepted work and never cancels the backend operation. Expand
and sending remain disabled until settlement. A pre-deletion failure instead
keeps the old conversation with its error and Retry restart action.

## Tests

| Acceptance | Planned evidence |
| --- | --- |
| .3-.7 | `config_chat_restart_test.go`: `TestConfigChatRestart` subtests for authorization, lifecycle exclusion, ordering, absence, duplication, and each failure stage |
| .3-.6 | `config_chat_restart_test.go` in handlers: `TestHTTPRestartConfigChat` request/ticket/response tests |
| .4, .6-.8 | `workspace-api.test.ts`, `use-config-chat.test.ts`, `use-quick-chat-resync.test.ts`, `quick-chat-sync.test.ts`: deferred restart, no replay, failure reconciliation, cross-hook admission and workspace/close races |
| .1-.2, .5-.9 | New `config-chat-panel.test.tsx`: disabled/confirm/cancel, mounted-session errors, progress and focus |

Test titles in the implementation must identify these scenarios; add AC
annotations where their mapping is not clear. Existing creation/resumption
tests are focused regression coverage, not evidence of restart implementation.

## E2E tests

- `settings/config-chat-restart.spec.ts`, project `chromium`: running and broken
  session replacement, cancel, failure/retry, new prompt, expansion, reload,
  and passthrough creation (AC .1-.8).
- `settings/mobile-config-chat-restart.spec.ts`, project `mobile-chrome`: same
  recovery value through the phone confirmation, busy/error reachability,
  focus return, header hitboxes, containment and no horizontal overflow (AC
  .1-.2, .5-.9).
- Include a narrow fine-pointer case and 767/768px boundary assertions in the
  desktop spec for the responsive header. Run existing desktop and mobile
  Configuration Chat specs alongside the new cases.

## Work orders

- [x] [Task 01: Backend replacement operation](task-01-backend-restart.md)
- [x] [Task 02: Header recovery flow](task-02-header-recovery.md)

Task 02 depended on Task 01. Both work orders are complete.
Execution was sequential in the primary session.

## Verification results

Design checks completed on 2026-09-29:

- `python3 scripts/list-docs.py validate`: passed (330 decisions, 1248 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/configuration-chat-restart`: passed;
  an explicit new-file whitespace and relative-link check also passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`, run with the installed
  Node 24 binary: actual docs-only changes are exempt; preflight against the
  planned panel change validates both work orders, requirement/AC definitions,
  design references, and manifest backlinks.
- `git status --short -- docs/specs docs/plans/configuration-chat-restart`:
  five new documentation files, unstaged and uncommitted.

Implementation checks completed on 2026-10-01:

- All 16 discovered backend restart test functions and their subtests passed
  with the race detector. Existing config-chat, quick-chat listing, empty-prompt
  and config-context startup regression suites passed. Scoped Go lint: zero issues.
- Eight focused frontend test files passed (123 tests). Type checking, scoped
  ESLint, locale completeness, generated catalogs and the new-copy ratchet passed.
- Fresh managed browser runs passed sequentially with one worker: 12 desktop
  tests and four phone tests, including the existing Configuration Chat flows.
  Both restart flows assert no conversation messages before the first new prompt.
- UI-01/UI-02 were checked against rendered desktop and phone screenshots.
  Phone controls and confirmation actions meet the 44px minimum, the drawer
  stays inside the viewport, and Cancel returns focus without losing a draft.
- Public documentation was updated. All 62 public-document validator tests
  passed and 47 published pages validated. The documentation catalog,
  specification lint, 36 specification-linter tests, delivery-package coverage,
  tracked diff whitespace and all 20 new files' whitespace checks passed.

The first final phone build encountered missing shared Go-cache files; a retry
with an isolated cache completed the backend but the host killed the frontend
build (exit 137). The successful fresh phone run used that isolated cache and
`NODE_OPTIONS=--max-old-space-size=2048`. These build failures occurred before
browser tests started. Exact test commands and RED/GREEN evidence are recorded
in the work orders. Changes remain uncommitted.

## Risks

- Normal deletion and stop responses permit background teardown; using them
  directly would not prove the required stop-before-replacement ordering.
- Stop can invalidate a deletion preview. Consume the ticket before stop and
  keep the subsequent deletion in the same accepted backend operation.
- Workspace changes, the setup `reset()` cleanup, or a late deletion event can
  discard a valid replacement unless identity and operation state stay separate.
- Runtime stop/start can fail independently. The frontend must distinguish an
  intact old conversation, cleared conversation, and retained failed replacement.
- Session lifecycle/cancellation lock ordering must be covered by deferred
  stop/resume tests rather than holding a broad lock across arbitrary callbacks.
