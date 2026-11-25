---
id: "01-typed-block-guidance"
title: "Preserve typed block-resolution guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.18
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.25
system_design:
  - ../../specs/agents/system-design/session-recovery-guidance.md
---

# Task 01: Preserve typed block-resolution guidance

## Summary

Preserve the canonical block classification when native Resume cannot resolve it.
Return the existing explicit continuation hint for supported causes without weakening recovery admission.

## In scope

- Re-read and classify an unchanged open block after failed native resolution.
- Preserve typed reason and current generation through error wrapping and WebSocket mapping.
- Include optional canonical `block_id` and `incarnation_id` in the existing details shape.
- Add canonical `unknown_prompt_outcome` to the existing continuation-reason mapping.
- Validate optional session and generation details at the frontend normalization boundary.

## Out of scope

Native proof repair, generation allocation, block removal, submission settlement, and issue 5256 implementation.
No browser composition changes belong to this work order.

## Acceptance

1. An unchanged supported block produces `session_restore_required` with its reason, target, generation, and `continue_from_history` action.
2. Replacement incidents, read failures, unsupported causes, and generic error text never produce stale continuation guidance.
3. Existing authenticated concurrent resolution still succeeds. No failed resolution changes payloads, unknown outcomes, journal state, or queue eligibility.

## Regression tests

Use `/tdd` after the explicit implementation request.
First add `TestRecoveryBlockResolutionGuidance` in the orchestrator package using the existing `nativeResumeFixture` pattern.
Cover `unresolved_durable_work`, `unknown_prompt_outcome`, a replaced incident, and failed canonical reads.
Assert typed reason and generation with `errors.As`, not message matching.
Preserve `TestNativeResumeAcceptsConcurrentAuthenticatedResolution`.

Add `TestRestoreRequiredRecoveryResponseRecoveryReasons` beside the existing handler regressions.
Cover all supported reasons, wrapped errors, and untyped/unknown refusals.
Add service-parser cases for foreign sessions, invalid identities, and invalid or mismatched generations.
Keep old optional-field responses covered.

## Verification

Run from the repository root after the workspace dependencies are installed.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestRecoveryBlockResolutionGuidance|TestNativeResumeAcceptsConcurrentAuthenticatedResolution' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator/handlers -run 'TestRestoreRequiredRecoveryResponse|TestWSLaunchSession_DurableRecovery' -count=1)
(cd apps/web && pnpm exec vitest run lib/services/session-recovery-service.test.ts)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_launch.go apps/backend/internal/orchestrator/session_recovery_guidance.go apps/backend/internal/orchestrator/session_recovery_guidance_test.go apps/backend/internal/orchestrator/handlers/handlers.go apps/backend/internal/orchestrator/handlers/handlers_durable_recovery_test.go
git diff --check
```

If the implementation does not need the proposed helper file, remove that file from the final lint command and results.
Do not run a command against a nonexistent path.

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_recovery_guidance.go` (new bounded helper)
- `apps/backend/internal/orchestrator/session_recovery_guidance_test.go` (new)
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_durable_recovery_test.go`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/lib/services/session-recovery-service.test.ts`

## Dependencies

None for typed feedback.
The actual continuation bootstrap remains dependent on issue 5256.

## Risks

A native-proof refusal can reflect a newer incident, not simply incomplete proof for the old block.
Only a matching canonical re-read can supply actionable context.
The hint describes an explicit recovery attempt and does not promise successful bootstrap.

## Parallelism

`sequential`

## Inputs

- [Typed server guidance and normalization](../../specs/agents/system-design/session-recovery-guidance.md#typed-server-guidance).
- [Active recovery criteria](../../specs/agents/requirements/session-recovery-failures.md), requirement 006.
- `native_resume_concurrent_resolution_test.go` and `handlers_durable_recovery_test.go`.

## Results

Completed 2026-11-11.

- Added a canonical re-read after every unsuccessful Resume resolution without a repository error, including generic metadata fallback and missing native-proof cases. Typed guidance is returned only for the same still-open supported block; replaced blocks, failed reads, and unsupported causes remain generic errors.
- Preserved block and incarnation identities plus generation in the bounded `session_restore_required` details. Added `unknown_prompt_outcome` to the existing action allowlist.
- Validated optional target identities and positive safe-integer generations in the frontend parser against the captured recovery target.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestRecoveryBlockResolutionGuidance$|^TestNativeResumeAcceptsConcurrentAuthenticatedResolution$' -count=1)`: passed.
- Generic fallback refusals with absent proof capability or nil proof retained bounded guidance for both `unresolved_durable_work` and `unknown_prompt_outcome`; tests confirm stored submission outcomes and admission blocking are unchanged.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator/handlers -run '^TestRestoreRequiredRecoveryResponse|^TestWSLaunchSession_DurableRecovery$' -count=1)`: passed.
- `(cd apps/web && pnpm exec vitest run lib/services/session-recovery-service.test.ts)`: passed, 31 tests.
- `bash scripts/lint-go-changed ...`: exceeded its five-minute timeout after reporting zero issues. The same scoped packages passed with `golangci-lint run ./internal/orchestrator ./internal/orchestrator/handlers --new-from-rev=bceeaf1c1d30ad05b953fd800b56ae46ebf5f2bf --timeout=10m --concurrency=2`: zero issues.
- `git diff --check`: passed.
