---
id: "14-attest-background-lifecycles"
title: "Attest supported background lifecycles"
status: done
wave: 14
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
---

# Task 14: Attest supported background lifecycles

- **Acceptance:** Only tested Claude and Codex paths can stamp typed
  background-work provenance; normalized presentation shape alone remains
  foreground-busy. Codex terminal child activities correlate to the original
  launch by child thread/session ID across tool-call IDs and retire it exactly
  once without turning collaboration controls into subagent cards.
- **Verification:** `cd apps/backend && go test -race ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator`.
- **Files likely touched:**
  `apps/backend/internal/agentctl/types/streams/tool_payload.go`, a focused
  background provenance file and tests,
  `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go`,
  `subagent.go`, their tests, and
  `apps/backend/internal/orchestrator/turn_activity.go`.
- **Dependencies:** None.
- **Inputs:** Spec adapter-attestation and Codex cross-ID scenarios; ADR 0049
  adapter trust boundary; existing Monitor attestation pattern and Codex
  correlation cache.
- **Worker:** Primary planner, direct implementation; standard model tier.
- **Output contract:** Report the typed provenance contract, supported adapter
  matrix, captured RED/GREEN wire fixtures for every terminal status, exact
  commands/results, risks, and update only this task status.
