---
created: 2026-09-10
status: implemented
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
legacy_specs: []
---

# Remote PR review checkout

## Outcome

Fresh remote workspaces honor an explicit branch or PR selection before agent startup.
One sequential work order implements the confirmed checkout defect.
The separate Files rendering report remains under investigation and is not an implementation-ready work order.

## Evidence and root cause

Task `88f9dfce-e50a-4fd5-8c59-59b6eef40add` selected PR 3527 on 2026-09-10.
Its repository row stores base `main`, checkout `feature/reap-agentctl-on-rem-b5w`, and PR number 3527.
GitHub reports the head repository as `nova28/kandev` and head commit as `6c5e4f809b10d25a1676f7ad0ae741a686bd6bc8`.
The session's Git status instead reports branch `feature/hello-88f9df` at base commit `544246e50e180dfde07596c252a4c99f94137061`.

Sprites calls `nonWorktreeTaskBranch`, which ignores `CheckoutBranch` when generating a branch.
Launch metadata also omits the explicit checkout branch and PR number.
The managed postlude can create a missing branch from HEAD and suppress failures.
Selecting the fork's branch name alone cannot repair this: it is absent from the base repository's normal branch refs.

## Approach and scope

Follow the selected-checkout section of the linked design.
Carry typed selection into shared remote preparation and use the base repository's PR head ref.
Keep explicit checkout strict and generated-branch behavior unchanged.
Preserve resume and editable contribution behavior.
No new schema, push permission, UI control, live task migration, relaunch, or deployment is included.

## Work orders

- [x] [Task 01: Honor selected remote checkout](task-01-selected-checkout.md)

## Verification strategy

The work order uses real Git to prove the full launch-metadata, script-resolution, and checkout path.
The PR fixture deliberately lacks the fork branch in origin heads.
Tests cover fresh, failed, and resumed workspaces plus shared executor compatibility.
No browser test is required for this backend checkout correction.
Public Git documentation must describe selected PR checkout without promising fork edit rights.

## Files panel investigation

The screenshot shows blank space above the final root entries.
Live diagnostics contain ResizeObserver loop warnings, but these do not prove causation.
A temporary browser fixture used the current `FileBrowserContentArea`, Radix ScrollArea, and production styles in an isolated app.
Scrolling, hide/show, viewport resizing, and shrinking 600 rows to 36 did not reproduce the gap.
The relevant file-browser source is unchanged between the observed backend commit and this worktree.
Do not implement a speculative measurement reset.

Next evidence needed: browser/version and the action immediately preceding the gap, especially task/tab switching or panel resizing.
Capture DOM row indexes, transforms, viewport dimensions, scroll offset, and a screenshot while the gap is present.
Once reproduced, amend the owning UI design and create a separate work order with desktop and phone regressions.

## Risks and handoff

- Shared script code affects Sprites, Docker, SSH, and Kubernetes.
- Existing custom scripts may already perform checkout; conflicting state must fail safely rather than reset user work.
- A PR can advance between selection and fetch. This contract reviews the fetched head, not a newly introduced immutable selection snapshot.
- Existing incorrectly created workspaces require a separate user-directed recovery; deployment alone must not reset them.

Implemented on 2026-09-10. The [work order](task-01-selected-checkout.md#results) records RED and GREEN evidence.
All task-defined checks passed. The previous origin-reference fix remains intact and uncommitted.
No live workspace was changed and no deployment occurred.
The Files rendering report remains open pending reproduction evidence.
