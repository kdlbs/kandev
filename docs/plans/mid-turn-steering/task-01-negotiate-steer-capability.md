---
id: "01-negotiate-steer-capability"
title: "Negotiate steer capability instead of matching agent names"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
---

# Task 01: Negotiate steer capability instead of matching agent names

- **Acceptance:** Steer/handoff eligibility is derived from the ACP `initialize`
  response (`agentCapabilities._meta.claudeCode.promptQueueing == true`), not
  from an agent id or persisted `agent_name`. An agent that does not advertise
  it is ineligible even when its name matches; an agent that advertises it is
  eligible without being named. The capability reaches the orchestrator over the
  existing `EventTypeAgentCapabilities` event and is recorded on the in-memory
  session activity record.
- **Acceptance:** A malformed or absent `_meta` (nil map, wrong types, non-bool
  value) fails closed to ineligible without panicking.
- **Verification:** `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/... ./internal/orchestrator/...`
- **Files likely touched:**
  `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
  (replace `supportsPromptHandoff`'s `agentID ==` comparison),
  `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
  (capability already stored at `a.capabilities`; emit the new field near :459),
  `apps/backend/internal/agentctl/types/streams/agent.go`,
  `apps/backend/internal/orchestrator/turn_activity.go` (replace
  `claudeBackgroundPromptHandoffEnabledForSession`'s `agent_name` comparison),
  `apps/backend/cmd/mock-agent/` (advertise the same `_meta`), plus tests.
- **Dependencies:** None.
- **Inputs:** Spec "API surface" (agent capability) and the plan's "Capability
  negotiation correction". ADR-0049's rejected alternative "Maintain a central
  agent-name whitelist" is the rationale. The pinned `acp-go-sdk` exposes
  `AgentCapabilities.Meta map[string]any`, so no SDK change is required;
  `promptQueueing: true` is confirmed present on bridge 0.49.0.
- **Risks:** This also re-gates the shipped `claudeBackgroundPromptHandoff`
  experiment. It only ever narrows eligibility, but the foreground-idle tests
  must keep passing unchanged — treat any change in their behavior as a defect
  in this task, not an expected consequence.
- **Output contract:** Report the capability-read helper's contract, the
  fail-closed cases covered, that both former name-comparison sites are gone,
  exact commands/results, and update only this task's status.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/... ./internal/orchestrator/...`: passed.
- Capability read: `acp/prompt_queueing.go` reads
  `agentCapabilities._meta.claudeCode.promptQueueing` once at `initialize` and
  caches it on the adapter.
- Fail-closed cases covered by test: nil `_meta`, wrong-typed `claudeCode`,
  non-bool `promptQueueing`, and absent key — all resolve to ineligible without
  panicking.
- Both former `agentID ==` name comparisons are gone; `grep -rn 'agentID ==' internal/agentctl/server/adapter/transport/acp/` returns no capability gate.
