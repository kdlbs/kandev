---
id: "05-acp-conductor"
title: "ACP conductor"
status: in_progress
wave: 5
depends_on: ["04-core-route-engine"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 05: ACP conductor

- **Acceptance:** Add the transparent execution resolver and dynamic conductor,
  reuse ACP resume only for the owning concrete profile, create fresh sessions
  across candidates, build bounded continuation, and fence late generations.
- **Files likely touched:** `apps/backend/internal/agent/runtime/{runtime,facade}.go`,
  `apps/backend/internal/agent/runtime/dynamic/**`,
  `apps/backend/internal/agent/runtime/lifecycle/{session,event_types,events}*.go`.
- **Dependencies:** Task 04.
- **Parallelism:** sequential.
- **Inputs:** Spec One logical chat, Threshold enforcement, Failure modes, and
  Persistence guarantees, Task 04 route state and current runtime lifecycle
  launch/resume contracts.
- **Output contract:** Report downstream-session ownership, continuation bounds,
  fencing behavior, files changed, exact commands and results, blockers, risks,
  and synchronized task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/...`
- **Risks:** A successor must never overlap an unfenced effectful predecessor or receive another provider's resume token.

## Results

In progress. Added the shared profile-execution resolver, dynamic conductor
contracts, route-aware resume lookup, and concrete-profile launch boundaries.
Task launches, prepared launches, resumes, and utility calls no longer pass a
virtual profile ID to lifecycle execution. The rollout-blocker repair adds
explicit output/effect evidence, bounded durable continuation persistence and
delivery, fail-closed mid-turn fallback, and bounded fallback across all
eligible candidates.

Verification:

- `go test -tags fts5 ./internal/agent/runtime ./internal/orchestrator ./internal/backendapp -count=1`

The command passed. The conductor is not yet the sole owner of lifecycle
launch, stop, event subscription, or restart reconciliation, so those broader
acceptance items remain open.
