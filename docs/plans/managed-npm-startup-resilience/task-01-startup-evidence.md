---
id: "01-startup-evidence"
title: "Preserve trustworthy startup evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.8
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Preserve trustworthy startup evidence

## Summary

Carry bounded process-exit and npm evidence across the agentctl initialization boundary.
Make pre-handshake failure distinguishable from session creation, restoration, or prompt failure.

## In scope

- Shared typed evidence, process generations, stop intent, and ordinary-versus-signal exit disposition.
- Strict npm transient/permanent code projection and exact-package ETARGET precedence.
- Optional wire fields, typed client errors, and legacy compatibility.

## Out of scope

- Starting retries, changing provider routing, new UI, or cache operations.

## Acceptance

1. Real child exits with empty stderr retain their generation and disposition. Local pipe errors alone do not become evidence.
2. Stopped, signaled, mismatched, unknown, and post-initialize cases cannot qualify as early setup failure. No raw secrets cross the new fields.
3. Old peers remain interoperable. Canonical npm diagnostics survive projection; permanent codes and policy errors take precedence.

## TDD and implementation

Add `TestManagedStartupEvidence` in a new process test file. Use owned subprocess helpers for exit 0, exit 1, stop, and platform-supported signal termination.
Add `TestManagedStartupDiagnosticCodes` in common/npmresolution; cover every allowed and denied code, legacy npm ERR! prefixes, prose false positives, and bounded input.
Add `TestInitializeStartupEvidence` across API/client tests and `TestInitializeFailurePhase` in lifecycle tests.
Assert that successful initialize followed by failing session/new or load is not labeled pre-handshake.
Test mismatched process generations before adapter access and absence of the field from legacy responses.

Capture exit evidence before events, but never wait for stderr or process drain while holding lifecycle locks.
Record the generation returned by start while preserving the existing Client.Start call contract.
Allow an explicit empty-stderr fixture; the existing restart mock defaults empty lists to ETARGET.

## Verification

Run from repository root:

```bash
(cd apps/backend && go test -race ./internal/common/npmresolution ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'TestManagedStartup|TestInitialize|TestStart|TestWaitForExit' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestInitializeFailurePhase|TestManagedRuntime' -count=1)
git diff --check
```

Record behavioral RED before production edits and GREEN afterward.

## Files likely touched

- `apps/backend/internal/common/npmresolution/npm_startup.go` and `_test.go` (new)
- `apps/backend/internal/agentctl/types/managed_startup.go` (new)
- `apps/backend/internal/agentctl/server/process/managed_startup_evidence.go` and `_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/managed_npm_stderr.go`
- `apps/backend/internal/agentctl/server/api/agent.go`, `server.go`, and new focused tests
- `apps/backend/internal/agent/runtime/agentctl/agent.go`, `client.go`, and focused tests
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and `session_startup_evidence_test.go` (new test file)

## Dependencies

None.

## Risks

Process status alone does not identify intent. Do not infer ordinary exit from an unknown exit code or a closed pipe.
Keep platform-specific signal interpretation behind existing process helpers.

## Parallelism

`sequential`

## Inputs

- [Design: Evidence and classification](../../specs/agents/system-design/managed-npm-runtime-recovery.md#evidence-and-classification).
- Existing process stderr/exit tests, Initialize API/client tests, and lifecycle session initialization.
- Scoped backend and agentctl AGENTS.md files; backend TDD guidance.

## Results

Completed 2026-10-02. The process manager now records generation-scoped ordinary, intentional, signal, and unknown exit evidence with bounded canonical npm codes. The initialize API rejects stale generations before adapter access and carries evidence only for the matching generation. Legacy responses remain valid; lifecycle phase errors wrap only ACP initialize failures.

RED: `go test ./internal/agentctl/server/process -run '^TestManagerProcessExitUsesSanitizedStderr$' -count=1` failed because the process exit event lacked bounded startup evidence; the transient-code projection regression also failed for every allowlisted transient npm code.

GREEN and final Task 01 checks:

- `(cd apps/backend && go test -race ./internal/common/npmresolution ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'TestManagedStartup|TestInitialize|TestStart|TestWaitForExit' -count=1)` passed.
- `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestInitializeFailurePhase|TestManagedRuntime' -count=1)` passed.
- Additional API boundary check: `(cd apps/backend && go test -race ./internal/agentctl/server/api -run 'TestHandleWSInitializeRejectsMismatchedProcessGenerationBeforeAdapterAccess|TestHandleWSInitializeCarriesExactProcessEvidence' -count=1)` passed.
- `git diff --check` passed.
