---
id: "12-plugin-permission-approvals"
title: "Approvals: permission requests and workflow review steps"
status: pending
wave: 6
depends_on: ["11-plugin-status-stop-commands", "09-plugin-outbound-mirroring", "15-host-delegated-human-responses", "16-host-step-approval-rpcs"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-008
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-008.1
  - AC-INTEGRATIONS-DISCORD-008.2
  - AC-INTEGRATIONS-DISCORD-008.3
  - AC-INTEGRATIONS-DISCORD-008.4
  - AC-INTEGRATIONS-DISCORD-008.5
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 12: Approvals (permission requests and workflow review steps)

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. **Permission requests.**
     - **Posting.** `permission_request.received.*` plus reconciliation with
       `Interactions().ListPending` post one prompt per pending permission
       interaction:
       - the title, and one button per option (at most 5 per row and 25 in
         total; any overflow links to Kandev);
       - the prompt is recorded in `task/<id>/interaction_prompts`;
       - duplicates post nothing.
     - **Resolving.** A button, `/approve`, or `/deny` from an operator first
       requests a human-response receipt for the operator's linked Kandev
       account (host task-15) with `{provider: "discord", subject: <user
       id>}`, the observed interaction revision, and the exact option. The
       plugin then calls exact `RespondPermission`. An unlinked operator gets
       an ephemeral deep link and `/link` instructions instead.
       - `/approve` picks `allow_once`, falling back to the first `allow_*`
         option.
       - `/deny` picks `reject_once`, falling back to the first `reject_*`
         option.
     - **Outcomes.**
       - Success: the prompt is edited to show the actor.
       - `CONFLICT`, a stale revision, or a resolved `Get`: "resolved
         elsewhere".
  2. **Workflow review steps.**
     - **Posting.** On `turn.completed` or `task_session.state_changed`, the
       plugin calls `GetStepApproval` (host task-16). When `awaiting`, it posts
       "Step *<from>* awaits approval → *<to>*" with Approve, at most once. It
       clears the prompt on `task.moved`.
     - **Approving.** Approve or `/approve` (with no pending permission) gets
       a `step_approval` receipt, then calls `ApproveStep`.
     - **Outcomes.**
       - `FailedPrecondition`: "nothing awaiting approval".
       - `Unimplemented` (office stage): "approve this stage in Kandev".
  3. **Missing grant or link.** Without the `human_responses` grant, every
     approval replies ephemerally that "a Kandev workspace manager must grant
     Discord approvals"; without a link, it replies with a deep link and
     `/link` instructions. Nothing changes in Kandev. With no argument, permission requests
     take precedence over the step. Autocomplete lists both kinds, and `/deny`
     never applies to steps.
- **Verification:** `make test-backend`
- **Files likely touched:** `server/commands/{approve.go,buttons.go,autocomplete.go,*_test.go}`, `server/outbound/{interaction_prompt.go,step_approval.go}`, `server/kandev/receipts.go`, `server/store/store.go`, `manifest.yaml` (`api_read: interactions`, `api_write: interactions`, the delegated grant).
- **Dependencies:** tasks 11, 09, 15, 16.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec "Slash commands", "Decisions" item 2, and the approval scenarios.
  - `docs/public/plugins-authoring.md` "Pending agent interactions" and
    "Exact execution controls and human interaction responses".
- **Risks:** If the maintainers pick identity linking for task-15, the receipt
  step becomes "resolve the linked Kandev user". Re-plan this task once the
  ADR lands.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
