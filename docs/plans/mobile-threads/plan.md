---
created: 2026-09-09
status: done
requirements:
  - REQ-UI-THREADS-DECK-003
  - REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/ui/system-design/mobile-quick-chat-topbar.md
legacy_specs: []
---

# Implementation Plan: Mobile Threads

## Overview

Implement the accepted full-width pager and task picker, then leave an isolated
instance running for evaluation. The user explicitly requested implementation
after the brainstorming turn. UI owns the existing responsive Threads contract;
the session delivery and selection boundaries remain unchanged.

The next evaluation follow-up corrects swipe-position feedback, then normalizes
Kanban/List phone headers to the compact Threads composition. Tasks 01-03 are
complete. The user subsequently authorized implementation of Tasks 04-05 and
an updated evaluation instance. Tasks 04 and 05 are complete.

## Scope

- Full-width phone conversations, compact page and task headers, and a thread
  picker using existing task summaries and mobile primitives.
- Localized labels, narrow viewport coverage, and a disposable fictional demo.
- Swipe-synchronous phone pagination and shared Kanban/List/Threads phone chrome.
- Preserve menu access to search, tools, status, metrics, and plugin actions.
- No backend contract, persistence, tablet/desktop layout, or session lifecycle
  change. Explicit default-Home selection is a separate subtask.

## Technical approach

Keep `ThreadsBoard` as the snap/viewport owner. Add one `MobileThreadPicker`;
each phone column's title opens it. Preserve stable task shells and the existing
session picker. Tasks 01-03 specialized the phone topbar for Threads. Task 04
exposes lightweight scroll-derived phone position from the existing viewport
owner. Task 05 replaces the other listing modes' phone-only header branches
with that shared composition and generalizes the menu action component.

### Follow-up diagnosis

`useThreadColumnActivation.updateVisibleIds` retains the previous Set when
intersecting IDs do not change. Its memoized nearest-column calculation then
does not rerun as the same two columns cross the midpoint. `ThreadsBoard`
currently passes `detailTaskIds` to its header, so pagination stays on the old
thread until that column exits the viewport. Transcript hydration is not the
direct timing gate.

A temporary Vitest fixture reproduced this with a 300-pixel board: A occupies
90% and B 10%, then A 40% and B 60%. Both stay intersecting. Expected B,
received A. The control assertion passes once A leaves and B fills the board.
The throwaway test was removed after diagnosis; Task 04 owns permanent RED
coverage of the new position signal.

UI owns both affected presentation contracts. The existing deck requirement
covers placement; new AC-UI-THREADS-DECK-003.13 specifies the missing during-drag
timing. REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001 is amended in place to supersede the
former fixed-brand/scrolling-strip layout. No new ADR is needed: session-stream
and navigation ownership remain unchanged.

## Tests

Run existing Threads selection, board, and activation tests for unchanged state
contracts. Use rendered geometry and interaction tests for the new presentation.
Task 04 adds midpoint/reversal, resize, membership removal, and cleanup cases
to `thread-column-activation.test.tsx`, plus header/picker projection coverage.
Task 05 updates `kanban-header-mobile.test.tsx` and adds focused generalized
menu-action tests. Work orders name exact regression cases and commands.

## E2E tests

`apps/web/e2e/tests/task/mobile-threads-view.spec.ts` covers
AC-UI-THREADS-DECK-003.8 through .12: full-width columns, title picker, local
content overflow, topbar reachability, swipe discovery, and one active
conversation after paging. It also checks phone/tablet transitions.
`threads-view.spec.ts` covers the preserved desktop deck.
Task 04 adds a mid-gesture assertion before touch release and destination chat
hydration (AC-UI-THREADS-DECK-003.13). Task 05 updates the shared phone-header and
plugin specs plus every affected search/chat/terminal entry-point test. It
covers REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001 across all three listing modes and
preserves existing activity/status tests. No gesture test may wait for a new
chat mount before checking pagination.

## Work orders

- [x] [Task 01: Improve the phone conversation deck](task-01-phone-deck.md)
- [x] [Task 02: Calm the header and reveal swiping](task-02-header-and-swipe-cue.md)
- [x] [Task 03: Move pagination into the topbar](task-03-inline-pagination.md)
- [x] [Task 04: Synchronize swipe-position feedback](task-04-swipe-position-feedback.md)
- [x] [Task 05: Normalize phone listing headers](task-05-shared-phone-topbar.md)

Execution order is 04 then 05, sequentially in the primary session. No native
implementation agents were authorized.

## Separate requested subtask

Kandev task `09730b6e-5e4c-4d81-ada6-fc0a5102e08a`, **Choose Threads as default
home view**, is a child of this task with `workspace_mode: new_workspace`,
base branch `main`, and inherited repository/profiles. It was created queued
with `start_agent: false`; no agent or extra session was launched. That task
owns explicit default-Home behavior and must distinguish it from remembered
last-listing state. Its workspace will not share this dirty worktree.

## Verification results

Tasks 04-05 are complete:

- 319 distinct unit tests passed across the combined Threads/listing suite and
  focused launcher/focus tests.
- 46 browser regressions passed: 33 mobile and 13 desktop, with one worker and
  retries disabled. These cover all three headers, held-touch pagination while
  loading, both opener focus paths, search reveal/clear, Home, plugins/metrics,
  Quick Chat activity/delivery, terminal reload/reuse/cleanup, status, and
  unchanged desktop Threads behavior.
- Permanent midpoint, held-swipe, shared-header, and detached-launcher
  regressions failed before their fixes and passed afterward. The phone
  viewport test now submits its bulk message once, avoiding replay of an
  accepted send. Terminal cleanup waits for the settled menu state.
- Typecheck, scoped zero-warning lint, formatting, i18n checks and ratchet,
  web build, 30 spec-linter tests, specification lint, and public-docs
  validation passed.
- The existing seeded demo was updated in place. All three headers measured
  56px high at both 360px and 393px, with 44px menu buttons and no document
  overflow. The final held swipe showed 2/4 at 65.8% scroll before release,
  then one active conversation after snap. Dark captures are
  `/tmp/kandev-mobile-threads-PuEDK5/mobile-shared-*.png`.
- HTTPS health and TLS-verified WebSocket upgrade passed during evaluation.
  After the user reviewed the demo, its backend and mock-agent descendants
  were stopped and only its private Tailscale Serve route was removed. Other
  instances and routes were preserved. The database and captures remain in
  the temporary demo directory.
- The user approved evaluation and requested a ready-for-review PR. Explicit
  default-Home selection and task-level context-menu access remain separate
  queued subtasks, outside this implementation.

### Historical results (Tasks 01-03)

- Implemented full-width phone pages, a title picker with focus return, a
  grouped page/view control, balanced two-line task titles, and inline topbar
  pagination with position and bounded page dots. Menu launchers and plugin
  actions remain reachable.
- 187 focused unit tests passed: 157 Threads tests and 30 shared-header tests.
- 22 browser tests passed with one worker and retries disabled: six mobile
  Threads, 12 desktop Threads, and four shared mobile-header/plugin tests.
- Web build, typecheck, scoped zero-warning lint, translation checks, public
  docs validation, and specification lint passed.
- The isolated branch instance is running privately at
  `https://koi.taile29c7d.ts.net:48490/threads` through Tailscale Serve.
  Its fictional workspace has four tasks, five sessions, and two demo profiles.
  Runtime ownership, seed IDs, screenshots, and stop instructions are in
  `/tmp/kandev-mobile-threads-PuEDK5/README.md`.
- Manual phone-emulation checks at 360 pixels verified equal viewport/chat
  widths, successful mock reply delivery, and secondary-agent selection.
  Physical-device keyboard and Safari behavior were not exercised.
- The header revision passed the same 187 unit and 22 browser tests. Its
  active-dot assertion was also rerun after being strengthened. Dark 360-pixel
  screenshots confirm balanced titles and contained multi-agent controls;
  the existing HTTPS route and TLS WebSocket upgrade remain healthy.
- The inline-pagination follow-up passed 162 focused Threads/page unit tests
  and all 22 browser regressions. Topbar height remains 56 pixels, while the
  two-line single-agent task header decreased from 139 to 111 pixels. Position
  follows the board's existing activation directly; no extra selection state,
  instruction label, or pagination row remains in the column header.

## Risks

- Long translated labels and nested code scrolling can pressure narrow layouts.
- Picker navigation must not change stable order, drafts, or selected sessions.
- Evaluation uses mocked agents in a fresh database, not production work.
- Moving menu launchers can break existing search focus, dialog handoff, Home
  routing, or background activity visibility. Task 05 preserves and tests each.
- Position sampling must stay independent of expensive chat activation and
  cancel pending frames/listeners on viewport or membership changes.
