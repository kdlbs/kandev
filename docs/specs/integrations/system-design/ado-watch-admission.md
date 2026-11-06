---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-ADO-WATCH-ADMISSION-001
created: 2026-11-06
owners:
  - kandev
---

# Azure DevOps Watch Admission Design

## Purpose and boundaries

This design repairs the provider reservation lifetime across the existing
asynchronous watch pipeline. The source is upstream main ce9918b094855c38e5b3b4c79c1b565fe5c379cb.
The existing [integration design](azure-devops-integration-02.md) owns provider
query, authorization and event publication. The task system's existing maximum
in-flight gate continues to own capacity; there is no second capacity ledger.

## Requirement mapping

| Acceptance criteria | Design section |
|---|---|
| AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.1, AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.2 | Reservation ownership |
| AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.3 | Task-created boundary |
| AC-INTEGRATIONS-ADO-WATCH-ADMISSION-001.4 | Compatibility |

## Confirmed failure

`azuredevops.Service.TriggerWorkItemWatch` reserves each matching item before
publishing `WorkItemWatchEvent`. `ReservationClaimed` makes the source skip a
second reservation. In `dispatchWatcherEvent`, a saturated
`acquireWatcherSlot` returns before the coordinator runs, without releasing the
provider's preclaimed reservation. Later queries return the item, but its
reservation prevents dispatch. Owner-context and dependency refusals are outside this bounded repair. Existing tests cover publish failure release, not this
capacity-refusal boundary.

## Reservation ownership

Keep the synchronous shared capacity gate and its pending counter. Introduce a
narrow source capability exposing whether this event owns a reservation claimed
before coordinator admission. Only Azure events carrying their existing
`ReservationClaimed` evidence opt in. A shared early-exit cleanup helper may
release that event's preclaim when no coordinator will execute; it must not call
ordinary `Release` for sources that have not reserved anything.

At the capacity-refusal branch, the generic wiring tests the optional source
capability `ReservationClaimed(any) bool`. Only a true result authorizes calling
that source's existing `Release` with the exact event. Use a cancellation-detached
context, matching dispatch's existing asynchronous context policy. The Azure
source logs release failures; an unsuccessful release must not be described as
successful settlement. Other early exits and historical pending claims are not
changed by this repair.

## Task-created boundary

After `CreateIssueTask` succeeds, retain the reservation even when
`AttachTaskID` fails. The created task is the authoritative no-duplicate boundary.
Keep watch ID, generation, project and item identities unchanged. Release uses
those exact existing identities. Do not release records belonging to a different
generation, item, or completed task.

## Compatibility

| Source/event | Required behavior |
|---|---|
| Azure work item, preclaimed | Release only its own preclaim on refusal before coordinator admission |
| Azure pull request, preclaimed | Same safe early-exit behavior where the shared helper is used |
| Other provider, no preclaim | Existing reserve/release ownership unchanged |
| Successful creation, attach failure | Keep deduplication; report association failure |
| Disabled watch | No poll dispatch or task creation |
| Missing cap | Existing unbounded admission |

No database schema, API DTO, UI, provider credential or deployment change is
required. Historical orphan claims remain an explicitly separate recovery concern.
The PR must not reset a watch or delete user tasks as a repair shortcut.

## Verification

Use a real temporary Azure watch store and task repository through the actual
source, dispatch gate and coordinator. Offer five matches at cap two, join each
dispatch through its final slot release, and assert two open tasks. Complete or
archive the first task, poll again, and assert item three now exists while the
open count stays two. A terminal task's retained deduplication must keep it from
being recreated. Cover claimed and unclaimed events for both Azure source types,
and an unclaimed non-Azure source. Retain the existing concurrent-burst tests.
No real Azure account, provider write, UI or runtime launch is required.
