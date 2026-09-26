---
id: "07-proposals-backend"
title: "Proposals decided: approve and reject backend"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-PROPOSALS-002
  - REQ-COORDINATOR-PROPOSALS-003
  - REQ-COORDINATOR-PROPOSALS-004
acceptance_criteria:
  - AC-COORDINATOR-PROPOSALS-002.1
  - AC-COORDINATOR-PROPOSALS-002.2
  - AC-COORDINATOR-PROPOSALS-002.3
  - AC-COORDINATOR-PROPOSALS-002.4
  - AC-COORDINATOR-PROPOSALS-002.5
  - AC-COORDINATOR-PROPOSALS-002.6
  - AC-COORDINATOR-PROPOSALS-002.7
  - AC-COORDINATOR-PROPOSALS-002.8
  - AC-COORDINATOR-PROPOSALS-002.9
  - AC-COORDINATOR-PROPOSALS-002.10
  - AC-COORDINATOR-PROPOSALS-002.11
  - AC-COORDINATOR-PROPOSALS-002.12
  - AC-COORDINATOR-PROPOSALS-003.1
  - AC-COORDINATOR-PROPOSALS-003.2
  - AC-COORDINATOR-PROPOSALS-003.3
  - AC-COORDINATOR-PROPOSALS-003.4
  - AC-COORDINATOR-PROPOSALS-004.2
  - AC-COORDINATOR-PROPOSALS-004.3
system_design:
  - ../../specs/coordinator/system-design/proposals.md
---

# Task 07: Proposals Decided, Backend (WP-5a)

## Summary

Add the approve and reject route handlers (task 01 declared their types) with
the claim, frozen spec, idempotent task creation, the reserved external-id
prefix and recovery, and publish `coordinator.updated` on every decision.
Needs only task 01's proposals table and store methods: tests insert pending
proposals through the store. Runs in parallel with tasks 02, 03 and 04.

## In scope

- In the decisions registration function of `backendapp/coordinator.go`:
  approve (from `pending` or `failed`, optional edits with the absent, null
  and empty-string rules of the proposals design's Edits table, re-validated;
  a failed attempt's `final_spec_json` is the base of the next), the
  create-outcome branches (`Created` then `SettleExternalID`, `FoundSettled`,
  `FoundUnsettled`), reject (from `pending` or `failed`, optional reason),
  zero-row completion re-read (200 with the current row, or 404 when the row
  is gone).
- Stale-claim recovery at startup, on approve, and on a proposal list or get
  by a `workspace.manage` caller only (added to task 01's read routes). The
  startup recovery hooks into task 01's decisions registration function (its
  startup-pass hook slot), not into the shared pass's call site.
- The `coordinator-proposal:` external-id prefix refused in the task service:
  `CreateTask` without `AllowReservedExternalID`, and
  `ReleaseTaskExternalID`, at the HTTP and MCP task create entry points.
- `coordinator.updated` after every committed claim, re-claim, completion,
  failure and reject, and never on a zero-row write. Task 03 publishes on
  propose; together they satisfy `AC-COORDINATOR-PROPOSALS-004.3`.
- 403 for approve and reject without `workspace.manage`; readers keep list
  and get (task 01).
- Whichever of tasks 03, 04 and 07 merges last into task 03's no-turn-start
  table adds the rows for the paths owned by the other two (task 07's
  proposal decisions and startup recovery here), so the table is complete
  regardless of merge order.

Reject is allowed from `failed` as well as `pending`, matching UI-03's failed
card. This departs from the source analysis plan (`implementation-plan.md`
revisions 9 and 10, outside this repository), which allows reject from
`pending` only, and is recorded in the
[proposals design](../../specs/coordinator/system-design/proposals.md#reject).

## Out of scope

- Any UI (task 08).
- Undo (phase 2 log), other proposal classes, reply with a condition,
  automatic classes, expiry.

## Mockup screenshots and scenarios

Screenshots: none. This work order is backend only; the proposal card
screenshots are cited by task 08.

Mockup scenario specs to port: none as Playwright (see the plan's
[Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)); the
decision behaviour of `05-rule-on-a-proposal` is covered by the Go tests below
and ported as Playwright in task 08.

## Acceptance

- One approval creates exactly one ordinary task in the target step with no
  agent started, under double approval, approve/reject races, a crash after
  the claim and a crash after the create.
- A pending or failed proposal is rejected through the API; every other
  decision returns 409 with the current proposal.
- No caller other than the coordinator service creates or releases a
  `coordinator-proposal:` external id.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/task/... -count=1
cd apps/backend && make lint
```

Go tests cover: double approve creates one task; edits re-validated (an
auto-start step refused); each Edits table row (absent unchanged, null 400
naming the field, empty title or workflow 400, empty step uses the start
step, empty repository clears it, a changed workflow without a step resets
to its start step), and edits to a failed attempt kept on the next approve;
each create outcome: `Created` settles and completes, identity lost completes
with the survivor, a settle not-found fails the proposal, `FoundSettled` and
`FoundUnsettled` complete with the found task and never settle or release
it; `coordinator.updated` published once after the claim and once after the
completion, and not on a zero-row write; a coordinator deleted during an
approval returns 404 with the task kept; a reader's list of a stale claim
writes nothing while a manager's list recovers it; the created task gets no
agent on a step whose `on_enter` has `auto_start_agent` (no
`auto_start_on_create` marker); crash after claim and after create recover to one
task; two readers of a stale claim, one wins, keeping the first
`final_spec_json` and `decided_by`; a stale original claimer's completion and
failure updates match no row (claim token); edits sent against an `approving`
row get 409; status is checked before edits (invalid edits against an
`approving` row get 409, an approve of an `approved` or `rejected` row whose
spec no longer validates gets 409, and a stale claim whose frozen spec no
longer validates is re-claimed without validation and fails at the create);
a claim, re-claim or reject that matches no row because the proposal was
deleted returns 404; create error to `failed` then approve again; reject
from `pending` and `failed`, 409 otherwise, status checked before the
reason (a 501-character reason on an `approved` row gets 409), and a
501-character reason on a `pending` and on a `failed` row gets 400 naming
`reason` with the row unchanged (`AC-COORDINATOR-PROPOSALS-003.4`); a reason
that is absent, JSON `null`, `""` or only whitespace stores SQL `NULL` and
returns `reject_reason: null`, a padded reason is stored trimmed, and a
non-string reason gets 400; a manager's `pending` list holding two stale
claims recovers both in list order before answering (both approved, one task
each), and when one row's recovery hits a store error the list still returns
200 with that row as stored and the other recovered; HTTP and MCP create
refuse the prefix, release refuses it, and the flagged internal create
succeeds; approve and reject by a reader get 403. These tests seed and assert
only this work order's own tables (coordinators and proposals seeded through
task 01's store); the `workspace.deleted` cascade that also removes proposal
and stall rows (`AC-COORDINATOR-COORDINATORS-006.1`) is task 04's subscriber
and its test, since task 07 does not depend on task 04.

## Likely files

- `apps/backend/internal/coordinator/{approve,reject,recovery}.go` and tests
- `apps/backend/internal/task/service/service_tasks.go`, `service_requests.go`, `external_id.go` (reserved prefix)
- `apps/backend/internal/backendapp/coordinator.go` (decisions registration function only)

## Dependencies

- Task 01 (proposals table, claim and settle store methods, types, event).
  While G0 is open the branch starts from task 01's branch and rebases onto
  main after each predecessor merges.

## Risks

- Recovery must never start a session; the no-turn-start table covers the
  recovery path.
- The task service's external-id idempotency must return the existing task
  rather than error; verify before relying on it.
