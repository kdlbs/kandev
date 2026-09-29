---
id: "03-codex-provider"
title: "Codex capability mapping and reconciliation"
status: complete
wave: 3
depends_on:
  - "02-observations-and-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-003
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-006
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-001.4
  - AC-AGENTS-BACKGROUND-WORK-002.1
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-002.3
  - AC-AGENTS-BACKGROUND-WORK-002.4
  - AC-AGENTS-BACKGROUND-WORK-002.5
  - AC-AGENTS-BACKGROUND-WORK-003.1
  - AC-AGENTS-BACKGROUND-WORK-003.2
  - AC-AGENTS-BACKGROUND-WORK-003.4
  - AC-AGENTS-BACKGROUND-WORK-004.1
  - AC-AGENTS-BACKGROUND-WORK-004.3
  - AC-AGENTS-BACKGROUND-WORK-006.1
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 03: Codex capability mapping and reconciliation

## Summary

Implement the first native provider against Codex 0.154.0. Establish capability evidence before exposing native controls, and normalize native discovery, child runs, and available output through the shared observation path.

## In scope

- Record a capability matrix in `protocol-evidence.md`: binary/schema revision, method/config support, fixture result, live result or not observed, and product fallback. The existing untracked `analysis.md` is not evidence.
- Extend typed client list params/response with cursor/limit/nextCursor and targeted background termination. Keep thread/item/process IDs separate from host PIDs and standalone command/exec process IDs.
- Implement all-page snapshots with deadline, generation fence, event-versus-snapshot ordering, one poller per connection, cleanup and no false disappearance on partial failure.
- Track child thread plus active turn identity, stable/nested parentage and multiple collaboration-call references. Isolated interrupt never falls back to root Cancel or a newly discovered successor turn.
- Normalize owned command-output deltas and final output without duplication; missing stream uses snapshot capability. Reconcile existing child messages, approvals, clarifications and usage bindings.
- Validate experimentalApi separately from feature config; inspect selected-version supported keys and effective policy before any per-session override. Keep stdin, arbitrary signals, restart and child steering unavailable for the initial Codex implementation.
- Use sanitized deterministic fixtures from the pinned schema. Add a disposable opt-in live scenario for background list/terminate and child interrupt; unobserved capabilities must be recorded accurately.

## Out of scope

New native providers, generic scheduling, host PID control, and production changes owned by later work orders. Preserve the design's explicit exclusions.

## Acceptance

1. `TestCodexBackgroundPaginationAndPartialFailure` and `TestCodexBackgroundSnapshotGeneration` prove no false completions from partial/stale snapshots and proper poller shutdown.
2. `TestCodexChildInterruptExactTurn`, `TestCodexChildBeforeBinding`, and `TestCodexNestedChildAndMultipleCalls` prove exact ownership, duplicate suppression and sibling/root survival.
3. `TestCodexOutputDeltaFinalReconciliation` and `TestCodexBackgroundCapabilities` cover stream/snapshot fallback, experimental opt-in, unsupported methods/input, operator restrictions and child permission resolution.

## TDD and verification

Write the named behavioral tests first and observe the relevant failure before implementation. Proposed file/test names below are implementation targets, not existing coverage. Run from repository root; fresh worktrees first install with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -race ./pkg/codexappserver ./internal/agentctl/server/adapter/transport/codexappserver)
(cd apps/backend && go test ./internal/orchestrator -run 'Background|Subagent')
git diff --check
```

## Files likely touched

- Existing: `apps/backend/pkg/codexappserver/types.go`, `client.go`, schema tests and pinned schema README.
- Existing: `apps/backend/internal/agentctl/server/adapter/transport/codexappserver/{adapter,activity,usage,approval_requests,user_input_requests}.go` and tests.
- Proposed: native adapter `background_work.go`, `background_work_test.go`, `background_work_output_test.go`.
- Existing opt-in harness `apps/backend/internal/agentctl/server/adapter/e2e/native_codex_test.go`; proposed `native_codex_background_test.go`.
- Proposed evidence: `docs/plans/agent-background-work/protocol-evidence.md`.

## Dependencies

Task 02: Durable observation and recovery path.

## Risks

Schema presence is not live evidence. Model-driven child spawning may not occur deterministically; do not turn a skipped live observation into a support claim.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/background-work.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/background-work.md), corresponding mapped sections.
- [ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Plan baseline and existing test patterns](plan.md#baseline-and-integration-points).

## Results

Implemented Codex 0.154.0 capability mapping, background polling, and child subagent reconciliation:
1. Extended `pkg/codexappserver` with paginated `ListBackgroundTerminals`, `TerminateBackgroundTerminal`, and `InterruptTurn` client methods.
2. Implemented `BackgroundWorkProvider` on Codex adapter (`apps/backend/internal/agentctl/server/adapter/transport/codexappserver/background_work.go`).
3. Added robust background polling with multi-page cursor fetching, partial failure preservation, and automatic poller teardown on empty list.
4. Implemented capability gating for terminals and subagents with explicit action availability states.
5. Recorded protocol evidence matrix in `docs/plans/agent-background-work/protocol-evidence.md`.
6. Verified with unit tests passing:
   - `TestCodexBackgroundPaginationAndPartialFailure`
   - `TestCodexBackgroundSnapshotGeneration`
   - `TestCodexChildInterruptExactTurn`
   - `TestCodexChildBeforeBinding`
   - `TestCodexNestedChildAndMultipleCalls`
   - `TestCodexOutputDeltaFinalReconciliation`
   - `TestCodexBackgroundCapabilities`
