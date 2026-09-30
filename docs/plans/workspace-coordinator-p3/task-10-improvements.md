---
id: "10-improvements"
title: "Improvement proposals and pending changes"
status: pending
wave: 5
depends_on:
  - "06-autonomy-ui"
  - "08-reply-with-condition"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-IMPROVEMENTS-001
  - REQ-COORDINATOR-IMPROVEMENTS-002
  - REQ-COORDINATOR-IMPROVEMENTS-003
  - REQ-COORDINATOR-INTEGRATION-003
  - REQ-COORDINATOR-INTEGRATION-006
acceptance_criteria:
  - AC-COORDINATOR-IMPROVEMENTS-001.1
  - AC-COORDINATOR-IMPROVEMENTS-001.2
  - AC-COORDINATOR-IMPROVEMENTS-001.3
  - AC-COORDINATOR-IMPROVEMENTS-001.4
  - AC-COORDINATOR-IMPROVEMENTS-002.1
  - AC-COORDINATOR-IMPROVEMENTS-002.2
  - AC-COORDINATOR-IMPROVEMENTS-002.3
  - AC-COORDINATOR-IMPROVEMENTS-003.1
  - AC-COORDINATOR-IMPROVEMENTS-003.2
  - AC-COORDINATOR-IMPROVEMENTS-003.3
  - AC-COORDINATOR-IMPROVEMENTS-003.4
  - AC-COORDINATOR-INTEGRATION-003.3
  - AC-COORDINATOR-INTEGRATION-006.4
  - AC-COORDINATOR-INTEGRATION-006.5
system_design:
  - ../../specs/coordinator/system-design/improvements.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 10: Improvement Proposals And Pending Changes (WP-12)

## Summary

Adds the `improvement` proposal kind and `propose_improvement_kandev`, the
improvement card with its evidence and diff, the approve branch that stores a
pending change, and the settings list where a manager applies or discards it.

## In scope

- Backend: the tool and its ordered validation, evidence checked against
  `coordinator_unattended_turns` and workspace tasks, the prompt paragraph in
  `prompt.go` ([Tool](../../specs/coordinator/system-design/improvements.md#tool));
  the approve branch and recovery
  ([Approve](../../specs/coordinator/system-design/improvements.md#approve));
  the pending-change routes with the base comparison and the phase 1 PATCH
  context write ([Pending changes](../../specs/coordinator/system-design/improvements.md#pending-changes));
  guard refusals; `coordinator_improvement_total`.
- The improvement `KindExecutor` registered in `registerKinds` with
  `ReRunsOnStaleClaim() = false` (a claim left `approving` settles `failed`
  with `outcome_unknown`; a manager's Approve re-runs the idempotent insert),
  the kind in `knownProposalKind`, `KindExecutors` and `kindAction`, the
  activity-only class `ActionImprovement` (not in `AllActions`), `recheckPolicy`
  returning nil for it, the `phase3` input of `ToolNames` and
  `actionClass` in `mcp/handlers/coordinator_authorization.go`, and its copy
  (`activityClassImprovement`, "No undo") in six locales
  ([Proposal statuses and kinds](../../specs/coordinator/system-design/integration.md#proposal-statuses-and-kinds),
  [Tool list](../../specs/coordinator/system-design/integration.md#tool-list)).
  The improvement is not a new column: phase 2's `kind` column takes the
  value `improvement`.
- `internal/coordinator/no_turn_start_test.go`: append the improvement
  approve and apply rows to `noTurnStartPaths`
  ([copilot](../../specs/coordinator/system-design/copilot.md#attended-only)).
- Web: `app/coordinator/proposal-card/improvement-card.tsx` branching from
  `ProposalCard`, "Runs behind it" through task 06's run read, the diff through
  the existing diff viewer, the approve gate and the reply control from task
  08; the settings "Changes waiting for you" list
  ([Card](../../specs/coordinator/system-design/improvements.md#card)).
- Copy in six locales.

## Out of scope

- Targets other than the context; automatic approval; editing an improvement.

## ASCII UI preview

See [plan UI-07](plan.md#ascii-ui-previews).

```text
| Improvement  [Changes coordinator context]                         |
| Runs behind it   09:12 completed 0.42 USD . Run record expired     |
| [Show the change]                                                  |
| [Approve as a reviewable change] [Reject] [Reply with a condition] |
```

## Acceptance

- The tool has no `in_reply_to` argument, and an improvement card never
  shows "Revised after your reply".
- A reply to a pending improvement (task 08's route) is delivered with the
  improvement text of [relay](../../specs/coordinator/system-design/relay.md#reply-delivery)
  and writes a `returned` activity row of class `improvement`; a
  `propose_task_kandev` call whose `in_reply_to` names a `returned`
  improvement is refused naming the field.
- A valid call stores one `pending` improvement with the server-read
  `context_before` and counts toward 25; each invalid field (including no run,
  a foreign run, a foreign task, 0 or 11 references, an unchanged context) is
  refused naming the field and stores nothing.
- An improvement is shown and counted with message, move and resume
  proposals, is governed by none of the six per-action settings (a
  `create_task` setting is never applied to it) and is never automatic; an
  approval left `approving` past the stale-claim window settles `failed` with
  `outcome_unknown` and no second pending change, and Approve on that card
  runs it again with still one pending change
  (`AC-COORDINATOR-INTEGRATION-006.4`, `006.5`).
- A conversation opened before phase 3 was on has no
  `propose_improvement_kandev` in its bound list, and while phase 3 is
  effective a call from it is refused as not in the profile and logged; one
  opened while phase 3 was effective has it. A call to the tool while phase 3
  is not effective, from a conversation whose list holds it, is the phase 1
  unknown-action error with nothing stored and no log row
  (`AC-COORDINATOR-INTEGRATION-003.3`, `AC-COORDINATOR-IMPROVEMENTS-001.1`).
- The card shows the evidence (expired runs as such), keeps Approve disabled
  until the diff is shown, has no Edit, and a raised `create_task` setting never
  approves an improvement.
- Approval stores one pending change and leaves the coordinator unchanged;
  Apply writes the context as a PATCH does (conversation replaced), is 409
  `context_changed` when the base differs, and a manager's context PATCH racing
  an Apply loses neither write: Apply holds the PATCH's per-coordinator lock
  from its first read, so the PATCH commits wholly before it (Apply is 409 and
  the edit is kept) or wholly after (SQLite, and PostgreSQL under
  `KANDEV_TEST_POSTGRES_DSN` with `-race`); the `AND context = ?` guard is
  tested at the store level with a mismatched base. Apply and Discard settle
  once under concurrency and are refused to readers and a coordinator
  principal; a change is 404 across coordinators and workspaces and while its
  proposal is not `approved` (a stale-swept `failed` proposal, or one rejected
  after that, leaves an inert change); with phase 3 off, improvements are
  hidden and the routes are unregistered (`AC-COORDINATOR-IMPROVEMENTS-001.4`,
  `003.4`).
- The `noTurnStartPaths` rows for improvement approve and for Apply, run
  through `TestCoordinatorConversationNoTurnStart`, assert that neither sends
  a prompt or starts an agent; Apply's conversation replacement starts no
  turn on the new conversation.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Improvement|PendingChange|NoTurnStart' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'PendingChange.*Race' -race -count=1
cd apps/backend && go test ./internal/mcp/... -run 'Coordinator' -count=1
cd apps/web && pnpm test -- app/coordinator app/settings/workspace
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/improvements.spec.ts
```
