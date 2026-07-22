---
id: "10-preserve-prompt-cycle-identity"
title: "Preserve prompt-cycle foreground identity"
status: done
wave: 10
depends_on: ["06-publish-completion-foreground-yield"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 10: Preserve prompt-cycle foreground identity

- **Acceptance:** Same-prompt `idle -> final output -> complete` releases
  foreground ownership and leaves registered detached work background-running;
  output does not impersonate a successor prompt; genuinely delayed predecessor
  idle/completion events still cannot yield an accepted successor.
- **Verification:** `cd apps/backend && go test -race ./internal/orchestrator/...`
- **Files likely touched:** `apps/backend/internal/orchestrator/turn_activity.go`,
  streaming handlers, lifecycle generation plumbing, and focused activity tests.
- **Dependencies:** Task 06 generation-aware publication.
- **Inputs:** Spec prompt-cycle requirement and the reproduced async-subagent
  ordering `idle -> final output -> complete`.
- **Output contract:** Report RED/GREEN event sequences, ownership model,
  publication cardinality, files, commands/results, risks, and update only this
  task status.
