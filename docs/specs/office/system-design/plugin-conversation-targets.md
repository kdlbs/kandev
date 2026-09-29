---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
created: 2026-09-25
owners:
  - kandev
---

# Plugin conversation automation targets system design

## Requirement mapping

| Requirement                     | Design section                                |
| ------------------------------- | --------------------------------------------- |
| `REQ-OFFICE-AUTOMATION-TARGETS-001` | [Destination contract](#destination-contract) |

## Purpose and boundaries

Office specifications own automation execution admission although implementation
lives under `internal/automation`. This design adds a generic destination that
uses the plugin-managed conversation transport. Plugin policy still chooses the
purpose, prompt, and schedule. No coordinator identity enters automation storage.

The [existing automation target modes](automation-target-modes.md) remain valid.
Existing `automation_run` and `normal_task` records preserve their behavior.
A new discriminated `managed_conversation` destination is explicit and cannot
inherit repository/workflow defaults from a task-producing destination.

## Destination contract

Persist installation identity, workspace ID, conversation ID, opaque instance key,
and destination revision. Validate same-workspace visibility, current approval,
supported input API, and live installation at save and firing time. The selector
shows plugin and instance display names with an availability reason. A schedule
cannot bypass revoked authority or implicitly reinstall a plugin.

Each admitted firing passes a stable automation occurrence ID to
`EnqueueManagedAgentInputExact`. Persist its receipt before marking delivery
accepted. The automation history distinguishes pending delivery, accepted, running,
completed, failed, paused, and uncertain outcomes by authoritative receipt readback.
A delivery acknowledgement is never a completion event. Duplicate firing attempts
reuse the same key; a changed payload under that identity conflicts. Snapshot the
resolved installation, plugin, logical instance, destination revision, and
conversation when admitting each occurrence. Retries and receipt reads must keep
using that snapshot, even if a human rebinds the schedule afterward. A temporary
receipt-read failure updates the observation but does not consume enqueue attempts
or terminalize an accepted run.

Plugin routine configuration uses proposed `ListManagedConversationSchedulesExact`,
`CreateManagedConversationScheduleExact`, `UpdateManagedConversationScheduleExact`,
`SetManagedConversationScheduleEnabledExact`, and
`DeleteManagedConversationScheduleExact` Host adapters. These require the separate
`host.v2.read:automations` / `host.v2.write:automations` capabilities and call the
shared automation service. Store installation ownership on plugin-created schedules;
exact writes require both ownership and schedule revision. Native human edits remain
available and increment revision. Deletion removes a schedule, never its destination.
Plugins cannot use these methods to edit another installation's schedule or create
an arbitrary worker-task automation. The method family covers the managed destination
only; broader automation administration is outside this package.

## Control flow and recovery

The automation evaluator admits the configured occurrence under existing overlap
rules. The destination adapter enqueues the prompt and stores the receipt reference.
A receipt observer reconciles status; event callbacks accelerate observation but are
not its source of truth. Restart resumes reconciliation from stored run records.
Do not create an additional disposable agent task to hold the shared conversation.

A paused destination retains accepted inputs without starting them. Disabling the
plugin blocks new delivery and produces a visible unavailable reason; retries use
bounded existing automation retry policy and the same occurrence identity. Missing
or uninstalled targets need explicit repair. Cancel a firing's unstarted input by
receipt ID; cancellation of an active turn requires exact generation checks.
Stopping or deleting an automation never deletes the conversation or other inputs.

## UI and portable configuration

Extend the target chooser with a managed-conversation option. Show an instance
selector instead of workflow/repository launch fields for that option. On phone,
use the existing full-height editor and bottom-drawer selectors. Display unavailable
destinations inline and keep the schedule editable. The history links to the
conversation and shows delivery status separately from agent outcome.

Portable export includes a plugin ID and logical instance key, never installation,
conversation, session, or receipt IDs. Import requires explicit destination binding
in the receiving workspace. Reject ambiguous or unsupported binding rather than
choosing the first instance. Existing exports and omitted target defaults stay
compatible. No cross-workspace authority is inferred from a portable file.

## Persistence, security, and observability

Use additive automation schema changes for the destination and receipt reference.
Reuse the host's managed-input tables; no second scheduling queue owns that input.
Host authorization runs again at dispatch. Preserve automation admission dedup and
cleanup ownership tests. Emit bounded destination-kind/status/reason metrics and
structured occurrence/receipt IDs. Do not include prompt bodies or integration
credentials in logs.

## Related documents

- [Requirements](../requirements/automation-target-modes.md)
- [Implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
- [Coordination platform decision](../../../decisions/2026-09-25-plugin-coordination-platform.md)
