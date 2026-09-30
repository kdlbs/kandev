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
  - REQ-COORDINATOR-SPEND-003
  - REQ-COORDINATOR-SPEND-004
  - REQ-COORDINATOR-CONTAINMENT-002
  - REQ-COORDINATOR-CONTAINMENT-003
  - REQ-COORDINATOR-INTEGRATION-007
  - REQ-COORDINATOR-INTEGRATION-008
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
  - AC-COORDINATOR-INTEGRATION-007.1
  - AC-COORDINATOR-INTEGRATION-008.1
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/wake-screens.md
  - ../../specs/coordinator/system-design/spend.md
  - ../../specs/coordinator/system-design/containment.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 06: Autonomy Strip, Held Item, Settings Section And Transcript Entry (WP-11)

## Summary

Makes autonomy visible: the autonomy read route, the strip on Needs you, the
held `autonomy` item, the settings Autonomy section with the ceiling,
spend and containment, and the "Woken by" transcript entry with its denied
permission count. Adds the public docs section.

## In scope

- Backend: `GET .../coordinators/:cid/autonomy`
  ([Autonomy read](../../specs/coordinator/system-design/wake-screens.md#autonomy-read)),
  `coordinator.updated` with `autonomy_changed`, and
  `GET .../coordinators/:cid/runs/:runId`
  ([Run read](../../specs/coordinator/system-design/wake-screens.md#run-read);
  the source of the "Woken by" entry and, later, of task 10's improvement
  card; built here with the autonomy read), including `last_turn.stop_state` (`stop_failing` when
  an open row's `stop_requested_at` is older than five minutes,
  [Stopping](../../specs/coordinator/system-design/spend.md#stopping)).
- Web: the autonomy input in `use-coordinator-inputs.ts`; the strip above the
  count strip; `attention.ts` emitting the `autonomy` item (kind rank 4, id
  `autonomy:<cid>`), counted in the coordinator count only, and the updated
  phase 1 ordering test for the amended `AC-COORDINATOR-NEEDS-YOU-002.4`
  ([Screens](../../specs/coordinator/system-design/wake-screens.md#screens)).
- Settings: the body of the Autonomy section. Task 01 already added the
  Sections entry (`autonomy` after `goal` in `coordinator-sections.tsx`, gated
  on phase 3 through `BASE_SLUGS`, with `sectionAutonomy` and
  `sectionAutonomyHelp` in six locales, and an empty `render`); this task
  replaces that `render` and does not touch the gating
  ([Settings layout](../../specs/coordinator/system-design/integration.md#settings-layout)).
  The body is the toggle and ceiling field with client
  validation and one-PATCH save, spend lines, containment list with fix lines and **Check
  again** ([spend Screens](../../specs/coordinator/system-design/spend.md#screens),
  [Settings display](../../specs/coordinator/system-design/containment.md#settings-display)).
- Transcript: the "Woken by" entry keyed on
  `metadata.coordinator_wake_turn_id`, filled from the run read
  ([Transcript](../../specs/coordinator/system-design/wake-screens.md#transcript),
  [Run read](../../specs/coordinator/system-design/wake-screens.md#run-read)),
  implemented as a renderer in the one message component so every surface
  that shows the conversation gets it
  ([Copilot everywhere](../../specs/coordinator/system-design/integration.md#copilot-everywhere)).
- Copy in six locales; `docs/public/coordinator.md` Autonomy section through
  `/docs-maintainer`. That page does not exist on this branch (the earlier
  phases' plans name it and it has not landed), so this task creates it with
  the Autonomy section and registers it in `docs/public/meta.json`, then runs
  `node scripts/validate-public-docs.mjs`.

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

- Strip states Active, each Held reason, degraded spend, unavailable
  with Try again, rendered from the autonomy read and refreshed on
  `coordinator.updated`; Needs you keeps its other items on a read error.
- The held item appears only for the five persistent reasons with at least
  one pending wake, orders after errors of the same age, counts toward the
  coordinator count and not the Inbox; the settings section shows ceiling,
  spend, 7-day mean, last turn cost and the four conditions re-read on open
  and Check again.
- The strip shows "Autonomy: Active (Waiting for the conversation)" for
  `conversation_busy` and "Autonomy: Active (Between turns until <time>)"
  from `admission.until` for `cooldown`, with no **Open settings** and no
  `autonomy` item; the read route returns `admission.until` only for
  `cooldown`. The strip has no "no ceiling" state (autonomy on requires a
  ceiling); "No ceiling" is a settings-section pill only.
- An open unattended turn whose ceiling stop was requested more than five
  minutes ago shows "Stop at ceiling not confirmed" with Stop on the strip;
  at four minutes it does not (`synctest` on the read). With autonomy
  turned off while that turn is open, the strip renders "Autonomy: Off" with
  the same warning and Stop; with autonomy off and no failing stop, it does
  not render.
- With phase 3 effective the Sections row shows Autonomy after Goal and the
  entry opens the section; with phase 3 not effective the entry is absent and
  `?section=autonomy` opens Identity (`AC-COORDINATOR-INTEGRATION-007.1`). The
  "Woken by" entry renders identically in the copilot panel, the popover and
  the conversation task view (`AC-COORDINATOR-INTEGRATION-008.1`).
- The transcript renders the unattended message as "Woken by N events" with
  the expandable list and the denied count, distinct from a manager message;
  at 390px in `en` with a held `containment` reason for `no_kandev_credential`, a last-woke age, a pending count and the spend pill, the strip's line 1 wraps to at most three lines (the stop-warning row is separate and not counted) with no horizontal scroll
  (`document.documentElement.scrollWidth <= 390`), the Stop and Open settings
  buttons are at least 44px tall, and the list toggle of the entry stays inside
  the message bubble. The e2e asserts these, not a screenshot comparison.
- The "Woken by" entry reads its count, list and denied count from the run
  read, never the message body; when the run read is a 404 it renders "Woken
  by events" with the message text behind the toggle. Two entries on one page
  cause one run read each, and a settled run is not re-read.
- The autonomy response carries `server_time`; `stop_state` is `stop_failing`
  only when the difference is strictly more than five minutes (exactly five is
  not), and an open page flips the strip without a reload through the timed
  re-read of [Screens](../../specs/coordinator/system-design/wake-screens.md#screens),
  and a page opened before the stop was requested learns of it from the
  `autonomy_changed` the ceiling-stop mark publishes.
  Stop is disabled while in flight and a failure shows "Could not stop the
  turn. Try again."
- The spend wire shape is one of three rows (measurable, unpriced,
  failed); a failed seven-day read shows "7-day daily mean unavailable", never
  0.00. An `Admit` result with detail `read_error` is a 500, not a hold reason.
- A server 400 on `cost_ceiling` shows a keyed string, never the server text;
  the decision toast's "Next" count includes the `autonomy` item; the held
  item's containment fix text is the settings display's, including detail
  overrides; a spend read failure is a 200 with `measurable` false.
- Saving turns autonomy on and sets the first ceiling in one PATCH; the toggle
  on with an empty ceiling keeps Save disabled with the required-ceiling hint.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'AutonomyRead|RunRead' -count=1
cd apps/web && pnpm test -- lib/coordinator/attention app/coordinator components/coordinators components/task/chat
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/autonomy.spec.ts
```
