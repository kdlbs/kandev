---
id: "11-managed-git-isolation"
title: "Isolate managed Git from repository-supplied code"
status: pending
wave: 11
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-GITISOLATION-001
  - REQ-AGENTS-GITISOLATION-002
  - REQ-AGENTS-GITISOLATION-003
acceptance_criteria:
  - AC-AGENTS-GITISOLATION-001.1
  - AC-AGENTS-GITISOLATION-001.2
  - AC-AGENTS-GITISOLATION-001.3
  - AC-AGENTS-GITISOLATION-001.4
  - AC-AGENTS-GITISOLATION-001.5
  - AC-AGENTS-GITISOLATION-001.6
  - AC-AGENTS-GITISOLATION-001.7
  - AC-AGENTS-GITISOLATION-001.8
  - AC-AGENTS-GITISOLATION-001.9
  - AC-AGENTS-GITISOLATION-001.10
  - AC-AGENTS-GITISOLATION-001.11
  - AC-AGENTS-GITISOLATION-001.12
  - AC-AGENTS-GITISOLATION-001.13
  - AC-AGENTS-GITISOLATION-001.14
  - AC-AGENTS-GITISOLATION-001.15
  - AC-AGENTS-GITISOLATION-002.1
  - AC-AGENTS-GITISOLATION-002.2
  - AC-AGENTS-GITISOLATION-002.3
  - AC-AGENTS-GITISOLATION-002.4
  - AC-AGENTS-GITISOLATION-002.5
  - AC-AGENTS-GITISOLATION-002.6
  - AC-AGENTS-GITISOLATION-002.7
  - AC-AGENTS-GITISOLATION-002.8
  - AC-AGENTS-GITISOLATION-002.9
  - AC-AGENTS-GITISOLATION-003.1
  - AC-AGENTS-GITISOLATION-003.2
  - AC-AGENTS-GITISOLATION-003.3
  - AC-AGENTS-GITISOLATION-003.4
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
---

# Task 13: Isolate managed Git from repository-supplied code

## Summary

Run every Kandev-managed Git command (backend and agentctl) without hooks, fsmonitor, unknown filters, textconv, external diff, merge drivers, or the `ext` transport by default, with a per-repository "Run Git hooks" opt-in.
Sandbox policies confine the agent process but not managed Git. A hook or filter that a sandboxed agent writes into the shared common Git directory would otherwise run later with Kandev's permissions, outside the sandbox.

## In scope

Execute the three ordered work orders of the [managed Git isolation plan](../managed-git-repository-code-isolation/plan.md): policy seam and default isolation, hooks setting persistence and propagation, settings switch and public guide.
Resolve integration with the sandbox launch path in favour of the structure on `sandbox/nono-plus`. Known integration points: the agentctl `executionCapabilities()` list (append `managed-git-policy.v1` outside the Linux-only sandbox branch), `CreateInstanceRequest` and the lifecycle create-request builders (`manager_launch.go`, `executor_docker.go`, `manager_execution.go`), the repository column lists and DTOs that enumerate `pull_before_worktree`, and `base_schema.go`.
Delivery is the follow-up PR that builds on kdlbs/kandev#4769.
Status ownership: the [managed Git plan](../managed-git-repository-code-isolation/plan.md) records results for its three work orders; this wrapper turns `done` when all three do.

## Out of scope

Changing what a sandbox provider enforces or reports, protecting the shared Git directory from agent writes, and agent-run Git. The detailed exclusions live in the requirements.

## Acceptance

- Every acceptance criterion in the frontmatter passes through the work orders named above, with the targeted Go, web unit, and "Run Git hooks" Playwright checks recorded in the managed Git plan.
- Sandbox-enabled and sandbox-free launches behave identically for managed Git. No sandbox identifier gates the policy.
- Existing sandbox tests still pass on the combined branch.
