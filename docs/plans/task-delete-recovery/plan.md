---
created: 2026-10-16
status: done
requirements:
  - REQ-TASKS-DELETE-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/task-delete-recovery.md
legacy_specs: []
---

# Task deletion recovery plan

## Overview

Allow a user to delete a task while keeping its workspace files when workspace
inspection fails. Deliver one complete vertical slice, including durable
retention and every cleanup consumer. A UI-only bypass cannot solve this bug.

## Evidence and classification

The incident task's empty `.kandev-workspace.json` caused
`decode workspace ownership marker: unexpected end of JSON input`.
Preflight returned 503 and the shared delete dialog disabled Delete. Direct MCP
deletion reached the same service rejection. Moving the damaged task root aside
allowed the existing delete path to finish. That manual recovery is complete.
Preserve the incident backup and do not use live tasks as test data.

This is a missing recovery behavior. Existing normal deletion deliberately
fails closed. The [conditional discard plan](../conditional-discard-confirmation/plan.md)
is completed historical work. Its generic inspection-error test becomes
mode-specific under this package. Normal deletion still fails closed.

## Scope

Explicit retain mode for HTTP/MCP task deletion and single, bulk, and cascade
UI flows. Include task-owned local and remote workspace resources, shared
environment safety, shutdown outcomes, storage maintenance protection,
confirmation reset, localization, and public recovery guidance.

Exclude SSD diagnosis, filesystem repair, archive behavior changes, a resource
browser, and a new purge action. This package does not add a force-remove flag.

## Technical approach

Follow the [vertical design](../../specs/tasks/system-design/task-delete-recovery.md)
and [decision](../../decisions/2026-10-16-retain-resources-on-task-delete.md).
Extend the existing deletion options, confirmation ticket, durable cleanup
snapshot, and inventory readers. Retain mode must reach task mutation without
physical inspection. It must also prevent later destructive cleanup.

Use one sequential work order. Backend, storage, and UI changes share one mode
contract and cannot ship independently. No subagent delegation is authorized.

Provider compatibility must be proved at the stop/cleanup boundary:

| Provider | Retained identity and behavior | Planned verification |
| --- | --- | --- |
| Local and worktree | Database paths, Git identities, and execution handles. Stop owned processes and keep directories, registrations, and refs. | Real temporary Git repositories, process-stop doubles, restart and storage integration tests. |
| Local/remote Docker | Recorded container, volume, and execution identities. Never remove file-bearing containers or volumes. | Provider tests distinguish stop from remove and exercise the missing-task inventory case. |
| SSH | Recorded host/environment and task-directory identities. Retain remote directories without contacting the host during admission. | Unreachable-host admission test and assertions that remote reclamation is never called. |
| Kubernetes and Sprites | Recorded provider resource and execution identities. Preserve pods, claims, and sandboxes that hold files. | Provider contract tests. If stop cannot be separated from destruction, report shutdown pending and invoke neither destructive operation. |

Unknown provider shapes retain all recorded resources and report shutdown
pending when safe stop cannot be proved. This does not claim provider-specific
stop support before the corresponding contract test passes.

## ASCII UI preview

UI-01: Delete dialog, workspace inspection failure, desktop.

```text
Current
Delete task
Task and cleanup consequences
Unable to check workspace changes. [Retry]
                             [Cancel] [Delete disabled]

Proposed, retain mode selected and confirmation ready
Delete task
The task and conversation will be deleted.
[x] Keep workspace files
    Workspace files and branches will remain.
[ ] Also delete 3 subtasks                    (if present)
             [Cancel] [Delete task and keep files]
```

UI-02: Same recovery action, phone centered inset alert.

```text
+------------------------------------+
| Delete task                        |
|------------------------------------|
| The task and conversation will      |
| be deleted.                        |
| [x] Keep workspace files           |
|     Files and branches will remain. |
| [ ] Also delete 3 subtasks          |
|------------------------------------|
| [ Delete task and keep files     ] |
| [             Cancel             ] |
+------------------------------------+
```

UI-03: Shared state transitions in the existing body.

```text
Normal check fails: Retry + unchecked Keep workspace files remain available.
Keep selected:     Confirming selection...  Action disabled.
Retain ready:      Delete task and keep files enabled.
Retain fails:      Specific error + Retry.  Action disabled.
```

Structure and order are required. Copy and ASCII spacing are illustrative.
Before retain is selected, the error says: "Unable to check workspace changes.
Retry, or keep workspace files to delete the task." Choosing keep-files clears
the old error and starts a database-only preview. It does not delete anything.
The normal discard option remains available only after a dirty normal preview.

Reuse the shipped centered task-delete alert. Its heading and footer stay fixed,
and its body is the only scroll owner. Phone actions stack and meet the 44px
touch contract. Desktop actions retain compact density. Respect dynamic
viewport and safe-area bounds. The existing card, sidebar, preview, and bulk
entry points all use the shared logic. No hover gesture is required.

UI-01 through UI-03 map to AC-TASKS-DELETE-RECOVERY-001.6 and .7. Verify long
translated labels, narrow fine-pointer phones, touch, and keyboard operation.
Do not add motion for a mode switch. Use existing tokens and primitives.

## Tests

Begin with a failing regression using an empty workspace marker. Assert normal
deletion refuses cleanup and retain deletion succeeds without filesystem/Git
calls. Prove task removal, retained resources, and shutdown tracking separately.
The work order lists the complete matrix and commands.

## E2E tests

Extend the real-backend delete fixtures in
`e2e/tests/kanban/card-menu-delete-archive.spec.ts` and
`e2e/tests/task/mobile-delete-discard-consent.spec.ts`. Reproduce the corrupt
marker in a disposable worktree. Delete through the UI and verify the task is
gone while the directory and its test content remain. A mocked 503 alone is
insufficient. Cover retention after a maintenance pass and backend restart in
backend integration tests, where those lifecycle boundaries are deterministic.

## Work orders

- [x] [Task 01: Delete tasks while retaining workspace resources](task-01-retained-deletion.md)

## Verification results

Implemented on 2026-10-16 after the user's explicit implementation request.
The shared dialog and HTTP/MCP deletion accept explicit retain mode. Durable
snapshots protect workspace resources after task deletion, including inventory
and stop handles from earlier cleanup jobs. Shutdown failure is a committed
deletion with a pending shutdown outcome.

Focused backend regressions, retention race tests, 75 frontend unit tests,
six desktop E2E tests, and five phone E2E tests passed. The phone recovery test
also restarts the disposable backend and checks the exact file hash, Git
registration, and branch identity. Type checking, changed-file lint,
localization, and documentation checks passed. See the
[work-order results](task-01-retained-deletion.md#results) for commands and limits.

The attempted broad Go sweep stalled in SQLite test-fixture schema setup.
Its completion is not claimed. The focused deletion, storage, repository, and
lifecycle checks passed. The production instance was not restarted or updated.

PR review remediation adds atomic publication of complete retention inventory,
shutdown before the physical-cleanup lease, and safe shared-local shutdown from
saved runtime identity. It also preserves the computed runtime status after
post-commit errors and omits empty retention IDs. Focused checks passed across
eight backend packages. Current-head CI and review resolution remain external
completion gates.

## Risks

- A forgotten cleanup consumer can remove retained files after task deletion.
  Inventory readers and pre-I/O guards must be tested, including container cleanup.
- Providers can combine agent shutdown with destruction of file storage.
  Retain mode must preserve the resource and report pending shutdown in that case.
- Retained resources consume storage or provider capacity without automatic expiry.
  Public guidance must state this and distinguish task deletion from disk cleanup.
- Snapshot reader compatibility and concurrent old cleanup claims are release
  gates. Missing retention evidence must never be read as permission to clean.

## Public documentation impact

Updated `docs/public/tasks-and-workflows.md` with recovery steps,
`docs/public/automation-and-mcp.md` with the explicit mode and outcome contract,
and `docs/public/git-operations.md` with the normal-cleanup distinction and recovery link.
The guidance states the retained storage/provider costs and the prohibition on
running older cleanup readers against retained snapshots.
