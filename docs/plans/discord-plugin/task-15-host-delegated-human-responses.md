---
id: "15-host-delegated-human-responses"
title: "Host: delegated human-response receipts for external chat surfaces"
status: pending
wave: 3
depends_on: ["01-host-control-rpcs-adr"]
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
---

# Task 15: Delegated human-response receipts

Repository: `kdlbs/kandev`. Use `/tdd`. Implement exactly the variant the ADR
(task-01) chose. The default described here is the delegated grant with an
external actor attestation.

- **Acceptance:**
  1. **The grant.**
     - A workspace manager can grant a declared Host v2 capability for
       delegated human responses (name fixed by the ADR, e.g.
       `host.v2.delegate:human_responses`) in Settings > Plugins > Workspace
       access, with a reason and an exact approval revision.
     - The grant shows in the audit history and is revocable. A plugin cannot
       grant itself.
  2. **Receipt issuance.**
     - With the grant, a plugin can request a receipt through a new Host call
       for:
       - one interaction ID;
       - its observed resource version;
       - the response kind and exact payload;
       - an attestation `{provider, subject, display_name}`.
     - Receipts are single-use, expire after 5 minutes, and are bound to the
       payload and revision, exactly like native receipts.
     - The existing exact `RespondPermission`/`AnswerClarification` accept the
       receipt unchanged.
     - Without the grant the call returns `PermissionDenied`. A stale revision
       returns `CONFLICT`.
  3. **Attestation storage.**
     - The resolved interaction (`Interactions().Get`) and the audit record
       carry the attestation, so the Kandev UI can show "approved by
       \<display_name\> via Discord".
     - Native receipts are unaffected.
- **Verification:**
  - Regenerate protos with the documented proto target.
  - `cd apps/backend && go test ./internal/plugins/... ./internal/backendapp/...` plus the package that owns response receipts (locate it from `POST /api/plugins/host/interactions/response-receipts`).
  - `cd pluginsdk && go test ./...`
  - `make -C apps/backend lint`
  - If the grant UI changes, run `cd apps && pnpm install --frozen-lockfile`, then `cd apps/web && pnpm run typecheck`, then the relevant vitest files.
- **Files likely touched:**
  - The response-receipt service and handler (find them from the route above).
  - The Host v2 grant ledger and capability vocabulary.
  - `pluginsdk/exact_execution.go` and `pluginsdk/interactions.go` (+ grpc, proto).
  - The plugin host implementation and backendapp adapter.
  - The interaction DTO and audit records.
  - Settings > Plugins > Workspace access labels, if new capability names need display text.
- **Dependencies:** task-01.
- **Parallelism:** sequential (proto, ledger).
- **Inputs:**
  - Spec "Host API additions requested" item 2.
  - `docs/public/plugins-authoring.md` "Exact execution controls and human
    interaction responses" and "Security and capabilities".
- **Risks:**
  - Security-sensitive. Add negative tests for: no grant; a revoked grant; a
    replayed receipt; a payload mismatch; a cross-workspace interaction; and
    a plugin whose manifest does not declare the capability.
  - If the ADR chose identity linking instead, re-plan this task before
    starting.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
