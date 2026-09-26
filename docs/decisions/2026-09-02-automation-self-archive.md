# ADR-2026-09-02: Permit Automation Self-Archive as Terminal Completion

**Status:** accepted
**Date:** 2026-09-02
**Area:** backend, agentctl, protocol, security, workflow
**Related ADRs:** [Let Users Configure Continuity, Not MCP Authority](2026-08-22-user-configured-automation-continuity.md), [Bind Automation Mutations to Event Targets](2026-08-09-bind-automation-mutations-to-event-targets.md)
**Related specs:** [Automation Continuity](../specs/office/requirements/automation-continuity.md), [Automation Target Modes](../specs/office/system-design/automation-target-modes.md)

## Context

The fixed `SurfaceAutomation` MCP profile must stop an automation from using
coordinator authority to mutate its own task, session, or blockers. The
automation still needs one terminal operation: a hidden run task must be able
to archive itself when its work is complete. The previous continuity decision
described every self-target mutation as invalid, which made the watchdog
completion path contradict the security contract.

## Decision

Allow `archive_task_kandev` to target the calling hidden automation task as its
terminal completion signal. The request still passes the normal workspace and
caller authorization checks and uses the normal task archive lifecycle.

Keep every other self-target restriction unchanged. Messaging, task updates,
moving, dependency changes, stopping, session spawning, and question or
permission discovery and resolution remain denied for the automation's own
task and its sessions. Foreign-workspace targets remain denied.

For a `github_pr_merged` run, the event binding remains authoritative. The
requested task must equal the persisted `automation_target_task_id`, so the
run task cannot use the self-archive exception to archive itself instead of the
merged pull request's target.

## Consequences

- Scheduled hidden automations can finish by archiving their own run task.
- The coordinator surface does not gain general self-mutation authority.
- The merged-pull-request flow retains an exact event-target security boundary.
- The archive handler and authorization tests must keep both the allowed
  scheduled self-archive and the rejected bound-run self-archive covered.

## Alternatives considered

1. **Keep all self-archive requests denied.** Rejected because a watchdog run
   cannot close its own hidden task through the existing agent action path.
2. **Add a native completion action outside MCP.** Rejected because it creates
   a second completion path and bypasses the existing archive lifecycle.
3. **Allow every self-target mutation.** Rejected because it would let a
   coordinator alter its own conversation, sessions, or blockers and would
   weaken the fixed-surface boundary.
