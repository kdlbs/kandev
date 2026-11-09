---
id: "01-safe-process-evidence"
title: "Preserve safe subprocess evidence"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.36
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.8
system_design:
  - ../../specs/agents/system-design/startup-process-diagnostics.md
---

# Task 01: Preserve safe subprocess evidence

## Summary

Preserve confirmed process status and a fixed mise trust-refusal marker through
agentctl initialization and safe child-log forwarding. Keep raw stderr and its
paths outside the new wire and log fields.

## In scope

- Add the neutral `common/startupdiag` recognizer and fixed marker representation.
- Recognize the reported complete mise signature before wrapper filtering.
- Extend `ManagedStartupEvidence` with optional diagnostic reason and completeness.
- Preserve exact-generation evidence through initialize responses and client decoding.
- Add validated process reason, disposition, and generation to the existing launcher log context.
- Keep npm completeness, bounded drain, protocol stderr consumers, and legacy decoding intact.

## Out of scope

Bootstrap UI classification, recovery policy, readiness error enrichment,
frontend changes, real mise execution, and arbitrary stderr forwarding.

## Acceptance

1. A real helper emits the refusal and exits during initialize. Both wrapper
   modes retain status and `mise_config_untrusted` for that exact generation.
2. Unknown, malformed, truncated, hostile, and stale diagnostics cannot produce
   trusted reasons or leak raw values. Exit zero remains distinct from missing status.
3. Backend forwarded logs retain only validated fields. Mise stderr remains
   nonempty non-npm evidence and cannot enable the empty-stderr retry path.

## TDD and regression cases

Begin with `TestStartupDiagnosticMiseTrustProjection` and
`TestHandleWSInitializeProjectsEarlyProcessExit`; run them before production edits.
Use the temporary reproduction described in [the plan](plan.md#confirmed-evidence)
as the setup pattern, rather than preserving its assertion about raw message wording.
The permanent API assertion inspects bounded structured evidence.

Add table cases for ANSI, paths containing spaces, unrelated prose, malformed
prefix/suffix, oversized lines, secrets, URLs, incomplete drain, and ring eviction.
Test ordinary status zero and one, intentional stop, signal, missing evidence,
and replacement generation before and after the evidence wait.
Use Go helper subprocesses instead of installing mise or depending on shell tools.

Add `TestPipeOutputPreservesStartupExitContext` and rejection cases beside the
existing child-log tests. Verify duplicate optional fields, unknown enums,
fractional and overflowing numbers, and raw `recent_stderr` exclusion.
The existing `TestPipeOutputPreservesSafeDiagnosticContext` must still pass.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/common/startupdiag ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/agentctl/launcher -run 'Test(StartupDiagnostic|StartupProcess|HandleWSInitialize|ManagedStartupEvidence|Initialize|PipeOutput)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/common/startupdiag/startup.go apps/backend/internal/common/startupdiag/startup_test.go apps/backend/internal/agentctl/types/managed_startup.go apps/backend/internal/agentctl/server/process/manager.go apps/backend/internal/agentctl/server/process/managed_startup_evidence.go apps/backend/internal/agentctl/server/process/startup_process_diagnostics_test.go apps/backend/internal/agentctl/server/api/startup_process_diagnostics_test.go apps/backend/internal/agent/runtime/agentctl/agent.go apps/backend/internal/agent/runtime/agentctl/agent_test.go apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context_test.go
git diff --check
```

The file list is the intended ownership set. If implementation chooses a sibling
file for the same boundary, update this command and record the exact final list.
Every changed Go test suite must remain in the focused race command.

## Files likely touched

- New `apps/backend/internal/common/startupdiag/startup.go` and `startup_test.go`.
- `apps/backend/internal/agentctl/types/managed_startup.go`.
- `apps/backend/internal/agentctl/server/process/manager.go` and `managed_startup_evidence.go`.
- New `apps/backend/internal/agentctl/server/process/startup_process_diagnostics_test.go`.
- New `apps/backend/internal/agentctl/server/api/startup_process_diagnostics_test.go`, using existing server helpers.
- `apps/backend/internal/agent/runtime/agentctl/agent.go` and `agent_test.go`.
- `apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go` and `child_log_context_test.go`.

## Dependencies

None. The wire additions remain optional, so this outcome can pass before
bootstrap classification and presentation support exist.

## Risks

Mise wording can change. Accept only the confirmed signature and return a safe
fallback for other shapes. Preserve the distinction between complete diagnostic
coverage and a genuinely empty npm diagnostic stream.

## Parallelism

`sequential`

## Inputs

- [Design: evidence and additive contract](../../specs/agents/system-design/startup-process-diagnostics.md#evidence-at-the-subprocess-boundary).
- [Recovery requirement 006](../../specs/agents/requirements/session-recovery-failures.md).
- [Managed startup requirement 004](../../specs/agents/requirements/managed-npm-runtime-recovery.md).
- `managed_startup_evidence_test.go`, `managed_startup_evidence_test.go` in the API package,
  and `launcher/child_log_context_test.go` provide the current helper and validation patterns.
- Read scoped backend/agentctl/API guidance and `/tdd` before permanent changes.

## Results

Implemented `internal/common/startupdiag`, the additive evidence fields, pre-wrapper projection, client validation, and the launcher allowlist. Added `TestStartupDiagnosticMiseTrustProjection`, `TestStartupProcessDiagnosticMiseTrustProjection`, `TestStartupProcessDiagnosticUnknownExitKeepsStatusOnly`, `TestHandleWSInitializeProjectsEarlyProcessExit`, `TestInitializeStartupEvidenceDropsMalformedOptionalFields`, and `TestPipeOutputPreservesStartupExitContext`.

Verification passed:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/common/startupdiag ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/agentctl/launcher -run 'Test(StartupDiagnostic|StartupProcess|HandleWSInitialize|ManagedStartupEvidence|Initialize|PipeOutput)' -count=1)`
- `bash scripts/lint-go-changed` for the final changed-file list
- `git diff --check`

Review remediation: added the prefix/suffix overlap guard with one-space and truncated-line regressions, pointer-based `exit_code` decoding for missing/null/zero, and malformed-npm-field invalidation while preserving exit evidence. Reran the focused race suite for the package set.
