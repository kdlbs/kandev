---
id: "06-autonomy-ui"
title: "Autonomy strip, held item, settings section and transcript entry"
status: pending
wave: 4
depends_on:
  - "05-delivery-unattended-turn"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-WAKE-005
  - REQ-COORDINATOR-WAKE-006
  - REQ-COORDINATOR-SPEND-004
  - REQ-COORDINATOR-CONTAINMENT-002
  - REQ-COORDINATOR-CONTAINMENT-003
acceptance_criteria:
  - AC-COORDINATOR-WAKE-005.5
  - AC-COORDINATOR-WAKE-006.1
  - AC-COORDINATOR-WAKE-006.2
  - AC-COORDINATOR-WAKE-006.3
  - AC-COORDINATOR-WAKE-006.4
  - AC-COORDINATOR-SPEND-003.4
  - AC-COORDINATOR-SPEND-004.1
  - AC-COORDINATOR-SPEND-004.2
  - AC-COORDINATOR-SPEND-004.3
  - AC-COORDINATOR-CONTAINMENT-002.3
  - AC-COORDINATOR-CONTAINMENT-003.3
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/spend.md
  - ../../specs/coordinator/system-design/containment.md
---

# Task 06: Autonomy Strip, Held Item, Settings Section And Transcript Entry (WP-11)

## Summary

Makes autonomy visible: the autonomy read route, the strip on Needs you, the
held `autonomy` item, the settings Autonomy section with the ceiling,
spend and containment, and the "Woken by" transcript entry with its denied
permission count. Adds the public docs section.

## In scope

- Backend: `GET .../coordinators/:cid/autonomy`
  ([Autonomy read](../../specs/coordinator/system-design/wake.md#autonomy-read)),
  `coordinator.updated` with `autonomy_changed`, and
  `GET .../coordinators/:cid/runs/:runId` (used by task 10; built here with
  the autonomy read), including `last_turn.stop_state` (`stop_failing` when
  an open row's `stop_requested_at` is older than five minutes,
  [Stopping](../../specs/coordinator/system-design/spend.md#stopping)).
- Web: the autonomy input in `use-coordinator-inputs.ts`; the strip above the
  count strip; `attention.ts` emitting the `autonomy` item (kind rank 4, id
  `autonomy:<cid>`), counted in the coordinator count only, and the updated
  phase 1 ordering test for the amended `AC-COORDINATOR-NEEDS-YOU-002.4`
  ([Screens](../../specs/coordinator/system-design/wake.md#screens)).
- Settings: the Autonomy section's toggle, ceiling field with client
  validation, spend lines, containment list with fix lines and **Check
  again** ([spend Screens](../../specs/coordinator/system-design/spend.md#screens),
  [Settings display](../../specs/coordinator/system-design/containment.md#settings-display)).
- Transcript: the "Woken by" entry keyed on
  `metadata.coordinator_wake_turn_id`
  ([Transcript](../../specs/coordinator/system-design/wake.md#transcript)).
- Copy in six locales; `docs/public/coordinator.md` Autonomy section through
  `/docs-maintainer`.

## Out of scope

- Answer in place, reply, automatic and improvements UI (tasks 07 to 10).

## ASCII UI preview

See [plan UI-01, UI-02 and UI-04](plan.md#ascii-ui-previews). Excerpt:

```text
Autonomy: Held (Cost ceiling reached) . Last woke 2h ago . 5 pending
  Spend 10.40 of 10.00 USD in 24 h  [Over]
| (bolt) Woken by 3 events    > KAN-418 question ...  1 permission denied |
```

## Acceptance

- Strip states Active, each Held reason, no ceiling, degraded, unavailable
  with Try again, rendered from the autonomy read and refreshed on
  `coordinator.updated`; Needs you keeps its other items on a read error.
- The held item appears only for the five persistent reasons with at least
  one pending wake, orders after errors of the same age, counts toward the
  coordinator count and not the Inbox; the settings section shows ceiling,
  spend, 7-day mean, last turn cost and the four conditions re-read on open
  and Check again.
- An open unattended turn whose ceiling stop was requested more than five
  minutes ago shows "Stop at ceiling not confirmed" with Stop on the strip;
  at four minutes it does not (`synctest` on the read). With autonomy
  turned off while that turn is open, the strip renders "Autonomy: Off" with
  the same warning and Stop; with autonomy off and no failing stop, it does
  not render.
- The transcript renders the unattended message as "Woken by N events" with
  the expandable list and the denied count, distinct from a manager message;
  the 390px layout matches the plan's phone views.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'AutonomyRead|RunRead' -count=1
cd apps/web && pnpm test -- lib/coordinator/attention app/coordinator app/settings/workspace
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/autonomy.spec.ts
```
