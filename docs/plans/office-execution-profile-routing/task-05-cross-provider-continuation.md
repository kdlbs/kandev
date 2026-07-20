---
id: "05-cross-provider-continuation"
title: "Cross-provider continuation"
status: done
wave: 4
depends_on: ["04-lifecycle-profile-ownership"]
plan: "plan.md"
spec: "../../specs/office/routing.md"
---

# Task 05: Cross-provider continuation

**Acceptance:** a classified post-start provider limit can requeue the same run on the next execution profile; profile changes clean up the old process and never reuse its native resume token; the task environment/worktree and Office identity remain stable; the replacement receives an explicit inspect-and-continue instruction.

**TDD cases:** Codex limit falls back to Claude Opus; Claude receives no Codex ACP token or config; same-profile recovery may resume; worktree/session ownership remains stable; ambiguous post-start failures do not fallback; exhausted routes park as before; continuation prompt references durable task/comments/status/git state without claiming chat transfer.

**Likely files:** Office scheduler routing lifecycle/error handling, orchestrator/lifecycle resume logic, `executors_running` recovery, Office prompt builder, route telemetry, and integration tests.

**Verification:** focused scheduler/orchestrator/lifecycle tests including process cleanup and resume-token guards.

**Dependencies:** Task 04.

**Output contract:** report fallback state transitions, token guard evidence, continuation behavior, tests, and provider-specific limitations.
