---
created: 2026-09-18
status: in_progress
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
legacy_specs: []
---

# Plan: Resume conversations on open without parking UI

## Overview

Opening a workflow-stopped conversation uses normal automatic recovery, including
older non-primary sessions. Remove the parked-session banner without replacing
it with another state the user must understand.

This replaces the earlier narrow exception for a reused primary session.
The user's 2026-09-18 instruction explicitly changes the parking policy.
[The new decision](../../decisions/2026-09-18-session-open-resumes-conversation.md)
records that change. The task system owns conversation activation and workflow
recipient isolation. No material product question remains unresolved.

Implement two sequential work orders: first correct recovery and ownership
handling; then remove parking presentation and verify the full desktop/phone flow.

## Evidence and conformance

The original investigation of task `62f15ae6-bb2d-40c5-863b-91366636114d`
confirmed `needs_resume=true` with `auto_resume_allowed=false` and
`ownership_unavailable` on build `f4c9131307`. A consumed workflow-switch stop
marker blocked recovery. An empty `deferred_launch: {}` was a second blocker.
Those branches originated in #3755; #3776 and #3779 corrected separate deadlocks.

The user then supplied task `efe289d6-a6cc-4f7b-9c9d-0c3014643270` and a
screenshot of the parked-session note. The user rejected the note and requested
resume on open even while a session is parked. The screenshot is presentation
evidence; this plan does not claim a new live-state investigation of that task.

The old inspection requirement 001.1 and parked-note requirement 003.3 are
explicitly superseded by 001.9 and 003.8. Criteria 001.7/8 retain restart and
settled-queue coverage. Queue identity, automatic admission, callback correlation,
and the deadlock fixes remain compatibility constraints.

## Scope

### In scope

- Remove workflow parking and historical stop markers as recovery restrictions.
- Treat absent, null, and empty settled deferrals as no pending launch.
- Resume only the selected conversation, without changing workflow ownership.
- Remove parked-session presentation, unused projection helpers, and locale copy.
- Audit policy-only parking writers/readers and remove unused code without migrations.
- Cover free/full capacity, pending destination, restart, preference, and delayed events.
- Update public recovery documentation when implementation ships.

### Out of scope

- Resending interrupted turns or settled workflow prompts on open.
- Changing workflow primary selection, automatic capacity limits, or manual overrides.
- Removing genuine launch queue status or background-work/Office parking.
- New session states, scheduler queues, migrations, or live-data cleanup.
- Relaunching explicitly cancelled, archived, or completed sessions outside existing rules.

## Technical approach

Use [Conversation recovery and workflow stop history](../../specs/tasks/system-design/queued-session-ownership.md#conversation-recovery-and-workflow-stop-history).
Remove parking-policy checks from `autoResumeEligibility`. Keep authorization,
resumability, archive/terminal rules, and nonempty deferred ownership validation.
Keep the general stop-intent parser and callback fences intact.

A queued destination must not get a duplicate resume. A selected sibling can
resume when capacity permits, while the original deferred recipient and prompt
stay unchanged. If capacity is full, do not replace accepted work with a sibling
resume record. Test the existing admission/conflict paths rather than creating
another queue. A queue or route change after status still requires guarded
validation. A new parking marker alone is no longer a denial reason.

Audit `workflow_parking` readers and writers across models, repository,
orchestrator, tests, and web. Remove policy-only code when unused; document any
retained independent purpose. Stored legacy markers become inert. Do not broaden
this cleanup to execution-stamped stop intents or unrelated parking concepts.

Remove `ParkedSessionNote` and `hasWorkflowParkingMarker` from
`launch-queue-status.tsx`, their caller in `task-chat-panel.tsx`, and their tests.
Remove `task:parkedSessionNote` from every catalog. Keep real queue status.

## ASCII UI preview

### UI-01: Desktop, earlier conversation selected

```text
Before
[Astra] [Luna] [Plan]
[This session is parked for the workflow. Opening it does not resume it...]
Conversation
Composer

After
[Astra] [Luna] [Plan]
Conversation (existing recovery/loading indication when needed)
Composer
```

When a genuine launch is queued, its existing task-level status remains visible.
No replacement parking row, badge, toolbar, or recovery instruction appears.
Session tabs keep their current location; conversation content owns scrolling.

### UI-02: Phone, earlier conversation selected

```text
Task header
[Selected session v]   -> existing Sessions picker
Conversation
Composer
[Chat | Plan | Changes | More]
```

The task drawer remains the entry point. The session picker selects the
conversation and normal recovery follows. The dedicated mobile layout, safe-area
navigation, and single chat scroll owner remain. No hover interaction is required.

UI-01 and UI-02 cover AC 001.9 and 003.8. Placement and absence of parking chrome
are requirements; text spacing is illustrative. Existing recovery errors, queue
waiting, loading, and explicit controls keep their established presentation.
The nearest mobile exemplar is `mobile-queued-session-ownership.spec.ts`.

## Test matrix

AC suffixes use `AC-TASKS-QUEUED-SESSION-OWNERSHIP-`.

| Case | Expected outcome | Criteria |
| --- | --- | --- |
| Primary or non-primary stopped conversation; valid or legacy parking; no pending work | Normal automatic recovery; no ownership transfer | 001.7, 001.9 |
| Consumed/unconsumed stop tombstone; malformed parking only | Parking history does not block recovery; callback correlation remains | 001.9 |
| Empty settled deferral, alone and with parking history | Recovery allowed; no settled prompt replay | 001.8, 001.9 |
| Same session already owns queued launch | No duplicate resume or prompt | 001.2; 002.4 |
| Different destination queued; selected sibling; free capacity | Selected conversation recovers; destination record and primary stay unchanged | 001.2, 001.5, 001.9 |
| Different destination queued; full capacity | No manual override or conflicting queue replacement | 001.2, 001.5; 002.6 |
| Auto-start prevention enabled | Open remains stopped; existing explicit Resume works | 001.5, 001.9 |
| Archive, explicit cancellation, completion, authorization failure | Existing lifecycle restrictions remain | 001.9 |
| Allowed status followed by conflicting queue or route change | Guarded admission rejects stale execution | 001.2, 001.6; 002.6 |
| Restart after switch or queue settlement | Conversation context survives; opening requests normal recovery | 001.7, 001.8, 001.9 |
| Desktop/phone with stored parking marker | No parked note; ordinary controls and real queue status remain | 003.8 |

## E2E tests

Update the existing desktop/mobile queued-session ownership specs and helper.
Replace assertions that require zero predecessor activation or a parked note.
Assert the selected conversation resumes without clicking Resume or sending a
message when preference and capacity permit. Capture `session_open` intent and
provider readiness; workspace readiness alone is insufficient.

Retain exact destination queue/prompt/primary assertions. Keep source message and
turn counts unchanged where open only restores provider context. Test free and
full capacity separately, including no manual override and no queue replacement.
Release only fixture capacity holders and assert one destination prompt delivery.

Use a workflow switch to establish real stopped-session metadata. Add restart
via the isolated `backend.restart()` fixture after leaving the target page.
Cover historical marker only, empty settled deferral only, and both together.
Verify same conversation identity and no prompt replay after recovery.

Use the existing phone task drawer and session picker at 393 px. Assert no
parking note, no document horizontal overflow, and usable existing controls.
Restore settings and runtime overrides in `finally`. No developer-instance restart
or direct production database mutation is authorized.

## Work orders

- [x] [Task 01: Restore ordinary recovery for workflow-stopped sessions](task-01-repair-session-open-eligibility.md)
- [ ] [Task 02: Remove parking presentation and verify user flows](task-02-remove-parking-presentation.md)

Order: 01 then 02. No subagents are authorized.

## Related delivery records

[Queued ownership](../queued-session-ownership/plan.md) is historical delivery
context. Its parked-suppression and parked-note scenarios are superseded by this
package. Exact queue ownership tests remain relevant. Keep historical verification
results and outstanding PostgreSQL checks unchanged; record new results here.

[Boot deadlock](../ceiling-boot-deadlock/plan.md) and
[replay/cancellation deadlock](../ceiling-replay-cancellation-deadlock/plan.md)
remain compatibility inputs. Do not change their lock order or claim fences.

## Documentation impact

Task 02 updates `docs/public/tasks-and-workflows.md` and
`docs/public/agents-and-profiles.md`: opening restores the selected conversation
under normal controls without changing workflow ownership. Remove instructions
that require a message solely because the workflow stopped the conversation.
No public behavior claim ships during this design-only turn.

## Verification results

Task 01 backend implementation is complete. Focused recovery, race, model/SQLite,
build, lint, and the full orchestrator package pass. The complete backend test
target still reports unrelated environment-sensitive failures in probe process
detection and home configuration discovery. Task 02 remains pending for the
frontend presentation removal and desktop/mobile E2E coverage.

## Risks

- Removing callback tombstones can let stale events affect a resumed execution.
- Opening a sibling can consume capacity, but must never steal its destination's queue.
- Hiding the note without changing eligibility leaves the reported defect intact.
- Tests that mistake workspace recovery for agent recovery give false confidence.
