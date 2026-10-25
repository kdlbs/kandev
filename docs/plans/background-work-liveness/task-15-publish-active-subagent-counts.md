---
id: "15-publish-active-subagent-counts"
title: "Publish active subagent counts"
status: done
wave: 15
depends_on: ["14-attest-background-lifecycles"]
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
---

# Task 15: Publish active subagent counts

- **Acceptance:** The live registry retains workload kind and provider child
  identity and derives exact session counts without a separate mutable counter.
  Session REST/boot/state/activity payloads expose `active_subagent_count`;
  task payloads sum it across sessions; count-only transitions publish.
  Duplicate/out-of-order terminal events and execution teardown cannot leave a
  stale count, and shells/Monitor watches never inflate it.
- **Verification:** `cd apps/backend && go test -race ./internal/orchestrator ./internal/task/...`.
- **Files likely touched:**
  `apps/backend/internal/orchestrator/turn_activity.go`,
  `event_handlers_streaming.go`, foreground/background accounting tests,
  `apps/backend/internal/task/dto/dto.go`, DTO enrichment and task aggregation
  tests, plus API v1 activity types if required by the existing DTO pattern.
- **Dependencies:** Task 14 typed provenance and terminal correlation.
- **Inputs:** Spec API/persistence/count scenarios; ADR 0049 derived-count
  invariant; existing foreground activity DTO/WS publication path.
- **Worker:** Primary planner, direct implementation; standard model tier.
- **Output contract:** Report registration schema, session/task aggregation
  semantics, fresh-load and count-only publication evidence, exact
  commands/results, risks, and update only this task status.
