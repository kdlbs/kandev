---
id: "01-host-control-rpcs-adr"
title: "ADR: plugin task control from external chat surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-001
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-001.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-001.2
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
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
[`2026-11-22-plugin-external-chat-task-control`](../../decisions/2026-11-22-plugin-external-chat-task-control.md)
is `accepted`. It records the maintainer decision on
[#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483):
`CancelTurn`; account linking gated by the `host.v2.write:human_responses`
admin grant, with receipts minted for the linked Kandev user; one receipt
model shared with #5202; `ApproveStep`/`GetStepApproval` on that receipt with
Office stages `Unimplemented`; `Message.Source`; `raw_content` kept in
`message.*` payloads with a docs correction; and the non-goals. Options (a)
alone and (c) are recorded as rejected alternatives.

- **Files:** `docs/decisions/2026-11-22-plugin-external-chat-task-control.md`;
  the spec and plan were reconciled. `docs/decisions/INDEX.md` is unchanged:
  it is a static entry page and the catalog is derived
  (`python3 scripts/list-docs.py decisions`).
- **Checks:** `git diff --check`; `python3 scripts/list-docs.py validate`;
  `python3 scripts/lint-spec-files.py --all`.
