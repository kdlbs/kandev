---
created: 2026-10-05
status: in_progress
requirements:
  - REQ-UI-BROWSER-DEMO-001
system_design:
  - ../../specs/ui/system-design/browser-demo.md
legacy_specs: []
---

# Browser demo delivery plan

## Outcome and scope

Deliver the browser-native demo from PR #1785 and retain compatibility with current application contracts.
The associated landing PR #169 consumes the release archive and exposes the demo entry point.
The scope includes seeded task histories, workspace data, workflow operations, system responses, and release distribution.
Real agent execution, external integration connections, and production backend behavior are excluded.

## Work orders

- [Task 01: Browser runtime and release compatibility](task-01-browser-runtime.md)

## Dependency order and risks

Build and publish the Kandev archive before deployment through the landing release-dispatch workflow.
Shared API contracts can change while the demo fixtures remain static. Typecheck and focused protocol tests detect this drift.
The demo uses existing UI surfaces, so upstream layout and navigation changes also require browser inspection.

## ASCII UI preview

### UI-01: Demo task session

Entry: Select a task from the demo board or sidebar. Criteria .2, .3, and .7 apply.

```text
+------------------+--------------------------+---------------------+
| Tasks            | Agent | Plan | PR        | Files | Changes     |
| Checkout timeout | User prompt              | acme-web/           |
| Audit logging    | Tool calls and response  | acme-api/           |
| React upgrade    | Approval or question     |                     |
| Empty states     |                          | Terminal            |
|                  | Message composer         | Simulated output    |
+------------------+--------------------------+---------------------+
```

The application retains its existing fixed navigation and panel scroll owners.
Phone layouts retain the application's separate task navigation and panel selection, rather than three side-by-side columns.
The sketch shows structural requirements, not exact spacing. Task preview stays closed unless the visitor enables it.

## Verification strategy

Run focused demo tests, typecheck, lint, translation checks, release-workflow tests, and the production demo build.
Use the existing frontend and E2E CI gates after the rebase.
Inspect the demo locally for the rendered behaviors described in UI-01 before release.

## PR fixup verification

The October 9–10 rebases preserve demo dispatch, contributor notification, and Windows signing contracts.
The fixup also restores queue status for tasks held only in the shared task store.
Cancellation fixtures hold settlement from the action boundary, including messages that arrive before the pending notification.
Continuation restart tests must observe the native prompt before the backend restarts.
The work order records local verification. CI remains pending until checks pass on the pushed head.

The pushed October 10 head exposed settings tests that predated profile ordering.
The fixup updates newest-first assertions and accepted-deletion epoch checks.
It also replaces the self-update page's incomplete store mock with its real provider.
The latest main now includes these settings fixes and marks accepted first-agent publication.
Independent profile events no longer prevent insertion of that accepted owner.
The upstream removal ledger and missing-owner guard protect deleted agents.

E2E shard 13 exposed a contradictory diff-fixture assertion left by the earlier rebase.
The deliberate preview-history fixture contains 14 files, so the obsolete two-file assertion is removed.
The upstream keyboard profile-order test polls geometry before asserting handle dimensions.
The work order records the failed CI job and requires zero-retry browser verification.

The upstream selector driver also uses Escape to dismiss desktop Quick Chat.
A failing trace proved the old Cancel lookup could not complete; the repaired case passed without retries.

## Current review repairs

The October 10 review repairs cover blocked storage, Worker failures, correlated handler errors, and saved file and plan edits.
Task workspaces now retain separate contents and select the correct repository seed.
Seeded clarification responses update the message and session. Imported workflows validate steps and events before mutation.
Simulated completion follows workflow move actions, and bulk moves publish each changed task.
The worker router now uses bounded modules without size or complexity suppressions.
Stable artifact uploads retry three times. Nightly builds and unsupported legacy backfills skip demo delivery consistently.
A new Stable release cannot silently omit the required demo build script.

The latest rebase includes the read-only Resume recovery fix from main.
Git fixture cleanup prunes missing worktree metadata before fetch and preserves live worktrees.
Mobile discovery routes finish before test teardown. Keyboard drag cancellation settles before the next activation.
The work order records the current validation and remaining delivery checks.
