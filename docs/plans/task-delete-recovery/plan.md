---
created: 2026-10-16
status: in_progress
requirements:
  - REQ-TASKS-DELETE-RECOVERY-001
  - REQ-TASKS-DELETE-RECOVERY-002
system_design:
  - ../../specs/tasks/system-design/task-delete-recovery.md
legacy_specs: []
---

# Task deletion recovery plan

## Overview

Keep the implemented retain-mode slice. Add normal task deletion with explicit
consent to unknown workspace changes and durable pending cleanup. Task 01 is
implemented. Task 02 is the revised design package requested on 2026-10-19 and
is implemented and locally verified. Task deletion must no longer depend on physical
workspace health after authorized cleanup obligations have been saved.

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
mode-specific under Task 01. Task 02 supersedes its unconditional normal-delete
inspection block with explicit consent, while preserving physical removal audits.

A fresh disposable Git-worktree reproduction on 2026-10-19 confirmed three
independent barriers: empty marker rejects inspection, the same marker rejects
cleanup even with discard consent, and an empty checkout `.git` rejects HEAD
capture. The temporary `TestReproDeleteIncidentEmptyMarker` passed all three
expected-failure assertions in 3.22 seconds and was removed after diagnosis.
No live incident data was used. This is a missing normal-delete contract,
recorded as REQ-TASKS-DELETE-RECOVERY-002.

## Scope

Explicit retain mode for HTTP/MCP task deletion and single, bulk, and cascade
UI flows. Include task-owned local and remote workspace resources, shared
environment safety, shutdown outcomes, storage maintenance protection,
confirmation reset, localization, and public recovery guidance. Task 02 adds
explicit unknown-inspection consent, partial physical preparation, independent
cleanup progress, outstanding-resource protection, and committed result fields.

Exclude SSD diagnosis, filesystem repair, archive behavior changes, a resource
browser, and a new purge action. This package does not add a force-remove flag.

## Technical approach

Follow the [vertical design](../../specs/tasks/system-design/task-delete-recovery.md)
and [decision](../../decisions/2026-10-16-retain-resources-on-task-delete.md).
Extend the existing deletion options, confirmation ticket, durable cleanup
snapshot, and inventory readers. Retain mode must reach task mutation without
physical inspection. It must also prevent later destructive cleanup.

Use two sequential vertical work orders. Task 01 already provides retention.
Task 02 extends normal deletion without weakening that feature. Its backend,
storage, and UI changes share one consent/result contract and ship together.
No subagent delegation is authorized.

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

### Task 02: Normal-delete recovery

Follow the normal-delete section of the linked system design. Extend existing
options, ticket binding, service admission, preparation, snapshots, workers,
HTTP/MCP adapters, storage inventory, the shared dialog and callback chain.
Keep physical removal audits mandatory. Do not globally swallow inspection
errors or infer success from missing owner rows.

For every provider in the matrix above, normal deletion saves database-owned
identities without requiring provider availability. Local/worktree cleanup
requires existing path/Git audits. Docker, SSH, Kubernetes, and Sprites cleanup
requires their existing exact ownership guards and shutdown dependencies.
Unknown or unsupported physical identity means pending/failed cleanup, never
unconditional destruction. Exercise each shared provider boundary with contract
fakes; do not claim live remote-provider verification from those tests.

## ASCII UI preview

The UI-01 to UI-03 drawings below record the delivered Task 01 slice. UI-04 and
UI-05 define the Task 02 extension. The retain choice remains unchanged.

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

Task 01 historical test strategy (Task 02 revises the normal-delete assertions):
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

### UI-04: Normal deletion, unknown inspection, desktop

Entry: existing single, bulk, or cascade Delete action.

```text
Before
Unable to check workspace changes. [Retry]
[ ] Keep workspace files
[Cancel] [Delete disabled]

After
Delete task
Workspace changes could not be checked. [Retry]
[ ] Keep workspace files
[ ] Delete even if workspace changes cannot be checked
    Local changes may be lost. Files that cannot be
    safely removed will remain pending cleanup.
[ ] Also delete subtasks                         (if present)
                                [Cancel] [Delete]
```

Delete is disabled until acknowledgement and its new confirmation resolve.
A transport/database error keeps the original error state and does not offer
this acknowledgement as a bypass. Known-clean/dirty states are unchanged.

### UI-05: Same state, phone centered inset alert

```text
+-------------------------------------+
| Delete task                         |
| Workspace changes could not be      |
| checked. [Retry]                    |
| [ ] Keep workspace files            |
| [ ] Delete even if workspace        |
|     changes cannot be checked       |
|     Local changes may be lost.      |
|     Files that cannot be safely     |
|     removed will remain pending     |
|     cleanup.                        |
| [ ] Also delete subtasks            |
| [             Delete              ] |
| [             Cancel              ] |
+-------------------------------------+
```

This short confirmation keeps the existing centered phone AlertDialog. The
heading/footer are fixed. The body owns scrolling with containment. The footer
clears the safe area and retains stacked 44px actions. Desktop keeps the 28px
row actions. Structure and reachability are requirements; ASCII spacing is
illustrative. Long localized copy wraps without horizontal overflow. Preserve
focus return and announce loading/errors. No new motion or color system.

Committed pending outcome: "Task deleted. Workspace cleanup is pending."
Show runtime shutdown pending separately when necessary. Retain still says
that files were kept. These views cover AC-TASKS-DELETE-RECOVERY-002.7-.9.

## Work orders

- [x] [Task 01: Delete tasks while retaining workspace resources](task-01-retained-deletion.md)
- [ ] [Task 02: Delete tasks when workspace inspection fails](task-02-unavailable-cleanup.md)

## Task 02 tests and E2E

The work order maps AC-TASKS-DELETE-RECOVERY-002.1-.9 to exact regression names,
files, and commands. Its first failing test must request normal deletion with
an unreadable marker and explicit unknown-change consent. Add a mixed healthy,
dirty, and corrupt scope. Prove task removal, independent cleanup, surviving
blocked inventory after restart, and retention-mode compatibility.

Extend the existing desktop card-menu and phone discard-consent Playwright
files with a real disposable empty-marker scenario. The task must disappear
with keep-files unchecked. The damaged directory must remain protected and the
response must report pending cleanup. Backend integration additionally restores
valid original evidence in the disposable fixture and verifies that retry can
finish. This proves pending cleanup is different from permanent retention.

## Verification results

Task 02 local verification passes: focused backend tests, service/retention race
checks, 79 frontend tests, seven desktop and six phone E2E tests, type checking,
scoped lint, translations, and documentation checks. PR delivery and fresh remote
CI/review evidence remain pending. The disposable diagnostic reproduction passed. Specification and package validation are recorded in Task 02 Results.
The results below are historical evidence for Task 01, not Task 02 validation.

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

- Cleanup can remain blocked when no trustworthy physical identity survives.
  Do not claim the pending outcome means every damaged file will be removed.
- Pending-cleanup protection must distinguish the exact owning worker from
  unrelated collectors without permitting a public bypass of retain mode.

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
Task 02 extends these pages with unknown-inspection consent and
pending-cleanup outcomes. Public documentation validation passes.
The existing guidance states the retained storage/provider costs and the prohibition on
running older cleanup readers against retained snapshots.
