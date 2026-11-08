---
id: "02-automatic-runtime-recovery"
title: "Recover tasks and discovery automatically"
status: done
wave: 2
depends_on:
  - "01-private-cache-preparation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.10
system_design:
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 02: Recover tasks and discovery automatically

## Summary

Use verified incomplete-cache evidence to prepare and replace the selected
runtime once. Cover tasks and every host/profile discovery entry point before
publishing terminal failure.

## In scope

- Task start/resume through the existing startup owner and replacement budget.
- Native Codex app-server initialize failures alongside ACP failures.
- Shared probe retry helper for startup discovery, profile baseline, and model options.
- Existing FAILED-session eligibility after restart, without database migration.
- Context supersession, admission ordering, safe final details, and current publication guards.

## Out of scope

- New automatic task-start policy, fresh conversations, or saved profile/runtime changes.
- Generic ENOENT retries, post-initialize retries, new controls, and requirement 005 eligibility.

## Acceptance

1. `TestManagedRuntimeCacheResume` fails first, then recovers ACP and native
   managed tasks with the same token and one prompt. Mixed healthy/recovered/
   exhausted siblings and delayed first-generation events cannot corrupt replacement.
2. `TestManagedRuntimeCacheProbe` covers agent-wide bootstrap, profile baseline,
   and model options for both protocols. Recovery preserves original context,
   draft/saved selections, admission bounds, and matching publication ownership.
3. Deadline, cancellation, cleanup refusal, context supersession, old peers,
   permanent failures, and an already-consumed retry budget prevent replacement.
   Preparation failure reports its actual safe cause and provider-start count.

## Verification

Run from the repository root. Preserve failing RED evidence before implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/orchestrator -run 'ManagedRuntimeCache|ManagedRuntimeStartup|ManagedRuntimeProbe|InitializeFailurePhase|TaskSessionStatus.*Failed' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'ManagedRuntimeCache|ManagedRuntimeStartup|ManagedRuntimeProbe' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility -run 'Profile|ResolveModelConfig' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go`, `managed_runtime_recovery.go`, and new cache-recovery tests.
- `apps/backend/internal/agent/runtime/lifecycle/session_startup_phase.go` and session initialization only where native transport proof requires it.
- `apps/backend/internal/agent/hostutility/manager.go`, `profile_probe.go`, and new shared retry helper/tests.
- `apps/backend/internal/agent/runtime/agentctl/client.go` for the opt-in start directive.
- `apps/backend/internal/agent/settings/controller/agent_discovery_test.go` for matching recovered projection.
- `apps/backend/internal/orchestrator/task_operations_orphan_resume_test.go` or a focused sibling status regression for existing FAILED sessions.

## Dependencies

Task 01 supplies verified evidence and the private owner. Do not implement a
second installer in lifecycle or hostutility. Future dependency-recovery work
extends this same retry helper.

## Risks

- Profile flights use detached request contexts. Supersession must cancel the
  flight without letting one HTTP waiter cancel other valid waiters.
- Native-protocol lifecycle support must not stop at utility-probe coverage.
- Process cleanup failure cannot be ignored to start a replacement.

## Parallelism

`sequential`

## Inputs

- Design: Task recovery; Startup discovery and profile recovery; Visible outcomes.
- Existing startup resilience, phase, generation, profile context, and publication tests.
- Requirement 006 and existing session resume eligibility in `GetTaskSessionStatus`.

## Results

Completed on 2026-11-07 with the same user-authorized implementation worker.

- All three exact commands above passed with `GOCACHE=/tmp/kandev-go-cache`.
  The first passed lifecycle, hostutility, settings/controller, and orchestrator.
  The race command passed lifecycle and hostutility. The full hostutility
  profile/model-option selection passed.
- `TestManagedRuntimeCacheResume` went RED before cache retry routing, then
  GREEN for managed ACP and managed native Codex app-server configurations.
  Production-shaped ENOENT evidence keeps `UnclassifiedNPMCode=true`; the test
  preserves the selected non-default version, original token, and one prompt.
- Agent-wide and profile baseline/model-option tests cover both configured
  protocols, exact attempt binding, unchanged launch context, and isolated
  profile publication. Supported native positives use no custom wrapper or
  unsupported CLI flags.
- Custom-wrapper refusal, cancellation of superseded in-flight preparation,
  and cancellation isolation between shared-flight waiters passed.
- The first full run caught a regression that required npm diagnostics for
  generic empty-stderr retry. Restricting that requirement to the verified-cache
  branch preserved the existing behavior; the exact rerun passed.
- Native lifecycle evidence uses the configured lifecycle path and mock
  agentctl transport. Task 03 adds real native Codex pre-initialize probe
  evidence and browser restart proof for an existing FAILED conversation.
- No plan/design edits, Git mutations, or live-instance changes were made by
  the implementation worker.
