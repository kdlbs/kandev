---
id: coordinator-goals-design
title: Goals and baselines design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-GOALS-001
  - REQ-COORDINATOR-GOALS-002
  - REQ-COORDINATOR-GOALS-003
---

# Goals and baselines System Design

## Purpose and boundaries

This design stores one active goal per coordinator with its frozen baseline
(ADR D21), gives the goal to the coordinator in its standing instructions
(D24), and adds the Goal section, the goal step of setup and the goal note
on Needs you. The measures read live task rows and the activity log; no
task history table is added.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-GOALS-001` | [Store](#store), [Routes](#routes), [Instructions](#instructions), [Goal UI](#goal-ui) |
| `REQ-COORDINATOR-GOALS-002` | [Goal note](#goal-note) |
| `REQ-COORDINATOR-GOALS-003` | [Baselines](#baselines), [Measures](#measures) |

## Store

`coordinator_goals`, both dialects:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | |
| `workspace_id` | text not null | |
| `name` | text not null | trimmed, 1 to 120 code points |
| `due_on` | text null | ISO calendar date `YYYY-MM-DD`, no time zone |
| `status` | text not null | `active` or `met` |
| `criteria_json` | text not null default '[]' | `[{id, text, done}]`, at most 10, text 1 to 200 |
| `baseline_json` | text not null | [Baselines](#baselines) |
| `set_at` | timestamp not null | when it became active |
| `met_at`, `met_by` | null | |
| `created_at`, `updated_at` | timestamp not null | |

Partial unique index `(coordinator_id) WHERE status = 'active'` enforces one
active goal. Rows are deleted with the coordinator and the workspace.

## Routes

Under `/api/v1/workspaces/:id/coordinators/:cid/`, phase-2 flag only:

| Route | Scope | Result |
| --- | --- | --- |
| `GET goal` | `workspace.read` | `{active: Goal \| null, last_met: Goal \| null, measures}` |
| `PUT goal` | `workspace.manage` | the active goal; body `{name, due_on?, criteria: [{id?, text}]}` |
| `POST goal/criteria/:crid` | `workspace.manage` | body `{done}`; the active goal |
| `POST goal/met` | `workspace.manage` | the met goal |

**PUT** runs in the per-coordinator locked transaction. Validation per
`001.1` (400 naming `name`, `due_on`, `criteria` or `criteria[i].text`). With
an active goal it updates name, due date and criteria: a criterion with a
known `id` keeps its `done`, one without gets a new id and `done=false`
(`001.2`). Without one it inserts a new active goal and computes the
baseline in the same transaction (`001.5`, `003.1`). When anything differs
from the stored goal, or a goal was created, it calls `resetConversation`
([permissions](permissions.md#conversation-reset)) (`001.7`).

**Criteria** updates only that criterion's `done` in `criteria_json` and
does not reset the conversation (`001.3`). An unknown criterion id is 404;
no active goal is 404.

**Met** sets `status='met'`, `met_at`, `met_by` with `WHERE status='active'`
and resets the conversation. With no active goal it returns the most recent
met goal with 200 and changes nothing (`001.4`).

A reader's write is 403 (`001.8`).

## Baselines

`baseline_json` is computed once, when a goal is created:

```json
{"open_tasks": 23, "approved_7d": 9, "rejected_7d": 2}
```

- `open_tasks`: tasks of the workspace that are not archived, whose state is
  not `COMPLETED`, that are not ephemeral and not origin `coordinator`, and
  whose workflow is watched by the coordinator at that moment
  ([permissions](permissions.md#watch-filter)).
- `approved_7d`, `rejected_7d`: from the activity summary function over the
  7 days before `set_at` ([activity log](activity-log.md#summary)).
- When the coordinator's `created_at` is later than `set_at - 7 days`, the
  two log measures are stored as `null` (`003.2`); `open_tasks` is always
  recorded.

The baseline is never recomputed; editing the goal keeps it (`001.2`).

## Measures

`GET goal` computes `measures` at read time with the same three definitions,
windows ending now, and returns for each `{current, baseline, direction}`:

| Condition | `direction` |
| --- | --- |
| baseline `null` | `none_no_baseline` ("No baseline") |
| `abs(current - baseline) < 2` | `none_small` ("No direction yet") |
| `current - baseline >= 2` | `up` |
| `baseline - current >= 2` | `down` |

(`003.3`, `003.4`). With no active goal, `measures` is `null`.

## Instructions

`prompt.go` adds, from the snapshot read at conversation-task creation, a
goal section: the name, the due date when set, and each criterion with
`[x]` or `[ ]`, between delimiters as operator text. With no active goal it
adds one line: "No goal is set for this coordinator." (`001.6`). A criterion
toggle does not reset the conversation, so the running conversation keeps
the done states it started with; the next conversation reads the new ones.

## Goal UI

`sections/goal.tsx` on the coordinator page:

```text
Goal
  Milestone   [ Ship the billing beta                     ]
  Due         [ 2026-10-31 ]
  Exit criteria
    [x] Invoices render for all plans           [remove]
    [ ] Stripe webhooks retried                  [remove]
    + Add criterion
  [Mark milestone met]
  Since this goal was set (2026-09-20)
    Open tasks         23 -> 19   down
    Approved (7 days)  No baseline
    Rejected (7 days)  No baseline
```

- Name, due date and criteria save through the settings save bar; a
  checkbox toggle saves immediately through the criteria route.
- **Mark milestone met** asks for confirmation, then posts.
- Readers see the values without controls (`001.9`).
- With no active goal the form is empty with **Set goal**.

The setup's What it is for step reuses the same form component without the
measures ([coordinators](coordinators.md#guided-setup)).

## Goal note

`components/goal-note.tsx` sits above the Needs you list while the phase-2
flag is on, fed by `GET goal` and refreshed on `coordinator.updated`:

| State | Note |
| --- | --- |
| no active goal, no met goal | "No goal is set, so this list is ordered by urgency alone." + **Set a goal** (managers) |
| active | "<name>" + "Due <date>" (or "Overdue since <date>" when `due_on` is before today in the viewer's time zone) + "N of M criteria met" (omitted when the goal has no criteria) |
| no active, last met | "<name> was met on <date>." + **Set the next goal** (managers) |

**Set a goal** and **Set the next goal** open the coordinator page at the
Goal section (`002.1` to `002.3`). The ordering of Needs you is unchanged.

## Security

- Writes need `workspace.manage`; the coordinator has no goal action.
- Goal and criteria text is untrusted display text and delimited in the
  instructions.

## Observability

Goal create, edit and met log at info with goal and coordinator ids.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
