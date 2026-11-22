---
id: "01-host-control-rpcs-adr"
title: "ADR: plugin task control from external chat surfaces"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
---

# Task 01: ADR for plugin task control from external chat surfaces

Repository: `kdlbs/kandev` (current `main`). Record the decision with
`/record decision`. This task follows maintainer feedback on the upstream
approval issue; do not start it before that feedback exists.

- **Acceptance:**
  1. A new ADR under `docs/decisions/` decides all of the following:
     - **`CancelTurn`** as an exact execution command
       (`api_write: execution` + `host.v2.write:execution`). It is fenced by
       the session resource version and the execution ID, delegates to
       `orchestrator.Service.CancelAgent`, and differs from `StopTaskRun`.
     - **Delegated human-response authority.** A workspace-manager grant in
       the Host v2 approval ledger lets a plugin obtain single-use human
       response receipts for one interaction revision and exact payload, with
       an external actor attestation `{provider, subject, display_name}`.
       The attestation is stored in the audit history and on the resolved
       interaction. The ADR also records why this was chosen over linking
       external identities to Kandev users (or records the reverse choice).
     - **`ApproveStep`/`GetStepApproval`**, reusing `ApproveSession` and
       gated by the same delegated authority. Office approval stages return
       `Unimplemented`.
     - **`Message.Source`**, carrying `metadata.source` only.
     - **No `raw_content`** in plugin deliveries of `message.*`.
     - **Non-goals:** interrupt-by-message, arbitrary moves, office-stage
       decisions, and plugin-minted receipts without a grant.
  2. `docs/decisions/INDEX.md` lists the ADR.
- **Verification:** `git diff --check`. The ADR is reviewed on the PR.
- **Files likely touched:** `docs/decisions/<next-id>-plugin-external-chat-task-control.md`, `docs/decisions/INDEX.md`.
- **Dependencies:** None. The maintainers' answer on the upstream issue is an input.
- **Parallelism:** parallel-safe with plugin tasks (different repository).
- **Inputs:**
  - Spec "Host API additions requested".
  - Plan "Investigation findings".
  - `docs/public/plugins-authoring.md` sections "Exact execution controls and
    human interaction responses", "Security and capabilities", and "Pending
    agent interactions".
  - The existing plugin ADRs (0043, 0047, 0048, 0050, and later ones listed in
    `docs/decisions/INDEX.md`).
- **Risks:** The delegated authority weakens the "only the native UI mints
  receipts" invariant. The ADR must bound it: an explicit grant, per workspace,
  revocable, single-use receipts, and an audited attestation.
- **Output contract:** summary, files changed, checks run, blockers, risks, and task/plan status update.

## Results
Pending.
