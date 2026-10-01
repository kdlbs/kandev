---
id: "07-agent-for-created-tasks"
title: "Agent for created tasks"
status: draft
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-CREATED-TASK-AGENT-001
  - REQ-COORDINATOR-PROPOSALS-002
  - REQ-COORDINATOR-PROPOSALS-005
acceptance_criteria:
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.1
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.2
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.3
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.4
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.5
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.6
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.7
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.8
  - AC-COORDINATOR-CREATED-TASK-AGENT-001.9
  - AC-COORDINATOR-PROPOSALS-002.13
  - AC-COORDINATOR-PROPOSALS-002.16
  - AC-COORDINATOR-PROPOSALS-002.17
  - AC-COORDINATOR-PROPOSALS-002.18
  - AC-COORDINATOR-PROPOSALS-005.11
system_design:
  - ../../specs/coordinator/system-design/created-task-agent.md
---

# Task 07: Agent for created tasks (WP 3.1-7)

## Summary

Owner decision of 2026-10-01 for live finding F-07: an approved or
automatically created task whose step, workflow and workspace name no agent was
a card that could never start. Each coordinator gets an "Agent for created
tasks" pair, the approval path adds it to the task only when nothing else names
an agent, and the proposal card says which agent will run. It depends on
nothing and is independent of work orders 01 to 06. It is **not** behind
`features.coordinatorPhase31`: it ships under `features.coordinator` alone, and
it changes no runtime flag, registry entry or `profiles.yaml`.

## In scope

- `internal/task/agentprofile`: `Resolve(Input) (string, Source)` with the
  orchestrator's order; `resolveTaskAgentProfile` loads its inputs and calls it
  (no behaviour change; `session_ensure_test.go` cases pass unchanged). If the
  architecture lint refuses that location, use the nearest leaf package both
  `orchestrator` and `coordinator` may import, and say so in the PR.
- `internal/coordinator` store: `task_agent_profile_id` and
  `task_executor_profile_id` (additive `ALTER`, then the two idempotent
  backfill statements on every start), row, DTO, create, setup and PATCH
  bodies, `Validator.ValidateAgentProfileFor` and `ValidateExecutorProfileFor`,
  `task_*_profile_status` on the read. A task-pair-only PATCH keeps the
  conversation and `config_revision`.
- Approve: the new call between `stepStillEligible` and `createApprovedTask`
  in `completeClaimedApproval` (no addition, workspace addition, pair
  addition, refusal, error), the
  narrow step, workflow-default and workspace-default readers and their
  wiring in `internal/backendapp/coordinator.go`.
- Proposal DTO `runs_with` on pending and failed rows, built by the same
  function as the approval.
- Web: merge rule 6 in `mergeProposal` (`use-proposals.ts`), so an equal
  `updated_at` row still refreshes a non-null `runs_with`; the `Proposal` type and `runs_with`, the card line (Needs you and
  transcript), the Agent for created tasks field group in the add form,
  Identity and guided setup's Who runs it, the pure `prefillTaskPair`, the
  Review row, status messages, copy in six locales (`pnpm run i18n:zh-hant`
  for the Traditional Chinese pair).
- Public docs `docs/public/coordinator.md`: the setting and the card line
  (`/docs-maintainer`).

## Out of scope

- Any change to when an agent starts (start-agent policy, automatic creates).
- A per-proposal or per-board agent; an executor stamp when only an agent
  profile resolves; a live preview while editing a proposal.
- Work order 06's fields. This order adds exactly one field group to Identity
  and the setup step, and one Review row, so the merge with 06 is mechanical.

## ASCII UI preview

```text
Identity
  Agent profile        [ Claude          v ]
  Executor             [ Local           v ]
  Agent for created tasks
    Tasks you approve start with this agent unless their board, workflow or
    workspace already sets one.
    Agent profile      [ Claude          v ]   (Workspace default)
    Executor           [ Local           v ]
Proposal card (Needs you and copilot)
  Pending Approval
  Add rate limiting
  Product . Backlog
  Runs with: Claude
  [Approve] [Edit] [Reject]
  (none resolves)  No agent available. Check Agent for created tasks in the
                   coordinator settings.
Review row  Agent for created tasks | Claude, Local | Identity | [Change]
```

## Tests that change, and why

- `TestAutomaticApproval_ApprovesAndRecordsRaiser`
  (`automatic_approve_test.go`) asserted the created request's `Metadata` is
  nil. It now asserts the stamp (`agent_profile_id`, `executor_profile_id`)
  when the fake chain is empty, no stamp when the chain yields a profile, and
  never `auto_start_on_create`. The amended `002.13` and `002.17` replace the
  "no profile on the created task" assertion.
- `TestAutomaticApproval_ManagerApprovalAfterFailureKeepsAutomaticAtAndAuthorization`
  (same file) asserted no `Metadata` at all to prove no agent starts. It now
  asserts the absence of `auto_start_on_create`, which is what `002.17`
  states.
- Approve tests that build `Service` over fakes (`approve_test.go`,
  `approve_race_test.go`, `decision_phase2_test.go`) gain a coordinator row
  with a task pair and a chain fake; their assertions are unchanged.
- `dto_test.go` `wantKeys` and the store and conformance fixtures gain the two
  fields; `coordinator-fixture.ts` and the guided-setup specs send them.

## Acceptance

- Chain: step, workflow default and workspace default each yield and win in
  that order; a step with a session target skips its profile and the workflow
  default; an empty chain stamps the pair; a chain that yields stamps nothing
  (neither agent nor executor); an unusable workspace default (missing in the
  workspace, passthrough) counts as empty; a proposal that starts an agent
  with only a usable workspace default stamps that profile alone (no executor)
  and an automatic or non-starting approval stamps nothing; a read error stamps
  nothing and leaves the row `approving` (500, and for an automatic approval
  the `unavailable` note and a counted row); a workflow that no longer exists
  is an empty input.
- Refusal: agent `missing`, agent `passthrough`, executor `missing` and a
  deleted coordinator each create no task and settle `failed` naming the setting,
  on the human path, the stale re-claim and the automatic path; approving
  again after fixing the setting creates the task; when the chain yields, an
  unusable pair is not read and does not block; both fields unusable names
  the agent profile.
- A found task of an earlier attempt is completed untouched; a racing second
  attempt returns the first task; an edit of the setting committed before the
  attempt's read is honoured, one committed after the read or after the create
  is not.
- Start policy: with a stamp, an approval starts an agent only if the policy
  allows it and an automatic approval starts none.
- Create, setup and PATCH matrices: absent, null, empty, whitespace, unknown
  profile, passthrough agent, unknown executor, cross-workspace profile, each
  400 naming the field; setup `step: identity`; PATCH absent unchanged;
  a task-pair-only PATCH keeps `conversation_task_id` and `config_revision`
  and publishes `coordinator.updated` once only when a stored value changes;
  an unchanged or absent task-pair field is not validated, so a coordinator
  whose stored task agent profile was deleted still accepts a context edit;
  a coordinator PATCH of its own pair still archives the conversation.
- `agentprofile.Resolve` cases added beside the orchestrator's: workflow
  default beats task metadata; a session-target step skips the workflow
  default.
- Migration: a phase 3 database with coordinators upgrades on SQLite and
  PostgreSQL; empty pairs are filled from the own pair, set values survive, a
  second start changes nothing.
- `runs_with`: each source, `none`, null on other statuses, null with a warn on
  a read failure, the list never failing; the card line for managers and
  readers, on Needs you and in the transcript, and its omission when null;
  `prefillTaskPair` matrix (workspace default usable, passthrough, missing,
  touched and untouched fields following the own pair).
  Also: `mergeProposal` rule 6 (equal `updated_at`, non-null incoming
  `runs_with` replaces, null never does, settled rows untouched); the agent
  field stays empty while `workspaceDefaultAgentProfileId` is `undefined`;
  approve with a step-resolved profile makes no workspace-default read; PATCH
  validation order (empty or null before check failures, own pair before task
  pair, agent before executor).
- Playwright: add a coordinator in a workspace with no defaults, approve a
  proposal, open the created task and see a session start without "no
  agent_profile_id"; the card line before approval; `mobile-chrome` at 390 px.

## Validation

- `make -C apps/backend test` for `internal/task/agentprofile`,
  `internal/orchestrator` (session_ensure tests unchanged) and
  `internal/coordinator`; store conformance and upgrade tests on SQLite and
  PostgreSQL.
- `cd apps && pnpm --filter @kandev/web` typecheck, lint and Vitest for the
  touched modules, `cd apps/web && pnpm run i18n:check`, and the Playwright
  spec of this order on desktop and `mobile-chrome`.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`.
