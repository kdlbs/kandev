---
created: 2026-09-12
status: complete
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
  - REQ-UI-INBOX-FAILED-001
system_design:
  - ../../specs/ui/system-design/needs-you-inbox-01.md
  - ../../specs/ui/system-design/needs-you-inbox-02.md
  - ../../specs/ui/system-design/needs-you-inbox-03.md
  - ../../specs/ui/system-design/inbox-failed-bucket-01.md
legacy_specs: []
---

# Implementation Plan: Needs-you Inbox

## Overview

Deliver a workspace-scoped sidebar destination that lists every clarification
bundle genuinely waiting on a human and lets the operator answer it without
leaving the page, per
[requirements](../../specs/ui/requirements/needs-you-inbox.md) and
[system design](../../specs/ui/system-design/needs-you-inbox-01.md) (part 2:
[control flow, failure, persistence, security](../../specs/ui/system-design/needs-you-inbox-02.md);
part 3: [visual and copy contract](../../specs/ui/system-design/needs-you-inbox-03.md)).

This feature was built through Kandev's own spec-driven-development workflow
(5 spec-review rounds, 4 build rounds, security/test/code review, an
operator-authorized direct fix round, then PR integration) with its running
record kept in the task's own plan store rather than as narrative here. This
document exists to satisfy the repository's PR documentation coverage gate
(`ci: enforce pull request documentation coverage`), which was not wired into
that workflow when this feature's spec canon was authored.

Task 03 adds the Inbox's second bucket, the failed-task bucket, per
[requirements](../../specs/ui/requirements/inbox-failed.md) and
[system design](../../specs/ui/system-design/inbox-failed-bucket-01.md). It
was explicitly out of scope for Task 01 and is delivered as a separate work
order against the same Inbox surface.

## Scope

### In scope

- Sidebar nav entry and route for the Needs-you Inbox, gated on
  `features.needsYouInbox`.
- A bounded, workspace-scoped list-and-count read path built on
  `clarificationBundleQuery` / `currentTurnAuthority`.
- In-place answer via the shipped `ClarificationPanelSection`.
- Per-user dismiss/snooze/restore sidecar that never mutates the underlying
  clarification record.
- The Inbox's second, Failed tab: a bounded list of terminal-`FAILED` tasks,
  deliberately excluded from the existing Needs-you badge count
  (Task 03, `REQ-UI-INBOX-FAILED-001`).
- i18n in all five shipped locales.

### Out of scope

Re-ask, the history bucket, cross-workspace rows, and any change to the
current-turn predicate's meaning. See the requirements documents' own
"Out of scope" sections for the full list.

## Work orders

- [x] [Task 01: Needs-you Inbox delivery](task-01-needs-you-inbox-delivery.md)
- [x] [Task 02: Inbox feature-toggle terminology](task-02-inbox-label-alignment.md)
- [x] [Task 03: Inbox Failed tab delivery](task-03-inbox-failed-tab.md)
- [x] [Task 03: Fix background-refresh loading state](task-03-fix-background-refresh-loading-state.md)

Task 02 depends on Task 01 and records the follow-up display-name alignment.
Task 03 depends on Task 01's shipped Inbox surface (tab strip host, route,
sidebar entry) but touches no file Task 01 did not already establish as
extensible; it is otherwise independent of Task 02's label change.

The background-refresh work also depends on Task 01 and fixes a
`resolveViewMode` / WS-coalescing regression against AC .19-.21; see its own
file for scope and verification.

## Verification results

See each work order for its full verification record. Summary: backend and
frontend build/lint/typecheck clean; targeted and full package test suites
pass (pre-existing, unrelated failures proven against merge-base in a scratch
worktree); desktop and mobile E2E specs for the answer-in-place and
failed-tab flows pass; `pnpm run i18n:check` and `pnpm run i18n:ratchet`
clean; `python3 scripts/lint-spec-files.py --all` passes.

PRs: https://github.com/kdlbs/kandev/pull/3641 (Task 01),
https://github.com/kdlbs/kandev/pull/3673 (Task 02),
https://github.com/kdlbs/kandev/pull/3683 (Task 03)

## Task 02 results

The follow-up label alignment keeps the stable feature identity, configuration,
route, and runtime behavior unchanged. The runtime flag and public configuration
reference now use `Inbox`, matching the destination copy contract in Part 3.

Follow-up PR: https://github.com/kdlbs/kandev/pull/3673

## Task 03 results

`resolveViewMode` could not distinguish a background refresh that followed a
successful read from one that followed a failure, so the Inbox re-flashed
"Loading..." on every WS/periodic refresh trigger and never resolved at all
with no active workspace. Fixed via a `lastAppliedOk` slice field that
survives the `status -> "loading"` overwrite a refresh performs, a
`hasActiveWorkspace` short-circuit to the empty view, and trailing-edge
queueing for WS refresh triggers that land inside the coalescing window.

Follow-up PR: https://github.com/kdlbs/kandev/pull/3685
