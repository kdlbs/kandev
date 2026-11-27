---
id: "02-gap-diagnosis"
title: "Diagnose actual receipt gaps during startup"
status: in_progress
wave: 2
depends_on:
  - 01-durable-ack
plan: plan.md
requirements:
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.2
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.3
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.5
system_design:
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
---

# Task 02: Diagnose actual receipt gaps during startup

## Summary

Verify the existing constructor-selected canonical generation stream. Detect
actual receipt coverage gaps and preserve the typed startup error through
readiness and recovery publication rather than hiding it behind an ACP timeout.
The incident does not establish a zero-cursor or stream-alias bug.

## In scope

- Production-path regression for constructor-selected stream/SQL cursor and
  authenticated descriptor coverage checks where needed for gap diagnosis.
- Explicit positive legacy compatibility and confirmed-empty initial streams.
- Typed stream readiness failure, replay-expiry propagation, and gap cause.
- Complete-inbox expiry recovery distinguished from missing receipt.

## Out of scope

- Skipping lost events, allocating successor generations, changing ACP
  capabilities, speculative bootstrap rewrites, new public recovery actions,
  UI markup, or local E2E.

## Acceptance

- The actual launch constructor carries the canonical generation stream into
  startup; its SQL cursor is used without a session alias. A constructor control
  must pass before any proposed identity rewrite. Patch identity selection only
  if a production-path failing regression independently demonstrates a defect.
- Missing or foreign identity/checkpoint and the 5494/5499 gap produce specific
  typed failures with no ACP initialize, prompt, cursor write, or legacy downgrade.
- Complete actual inbox coverage resumes ordered projection; positive legacy
  and genuinely fresh durable sessions still initialize normally.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/orchestrator -run 'Test(DeliveryGapDiagnosis|DurableRetentionBootstrap|AuthenticatedDeliveryReplay|DurableAdoption|DeliveryStartupFailure|AgentStreamReplay)' -count=1)
git diff --check
```

Confirm new gap/error regressions are discovered and record their pre-fix
failure. Record the constructor identity control separately; do not claim it
failed if it already passes. Task 04 owns CI-only UI verification.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/agent_delivery_retention.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_gap_diagnosis_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/orchestrator/session_launch.go`

## Dependencies

01-durable-ack.

## Risks

The updates stream transports ACP control responses. Any new gap preflight uses
the authenticated HTTP descriptor without waiting for ACP negotiation. Preserve
the existing verified stream identity and avoid competing consumers/deadlocks.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Design: stream identity, compatibility, typed diagnosis, and loss detection.
- Existing stream readiness, retention, adoption, and launch-guard tests.

## Results

Authenticated pre-ACP descriptor validation, canonical actual SQL cursor use, typed startup failure propagation, and real-inbox prefix projection implemented. Focused lifecycle regressions pass; new explicit compatibility controls require final verification. No bootstrap alias rewrite was needed.


### CI startup compatibility correction

Docker and SSH managed-runtime retries exposed retained child-exit events before native initialization creates the first SQL generation/checkpoint. A focused regression reproduced the checkpoint refusal. The preflight now permits zero-cursor replay only for explicit fresh generation-one bootstrap, fully retained unacknowledged output, no native conversation/replacement, no retained prompt work, no SQL cursor and no committed native generation. Restored ownership and genuine pruning still fail closed. The focused bootstrap/preflight/managed-startup and newly integrated archived-Codex regressions passed with the current main change. The Docker/SSH scenarios retain their assertions and attach backend logs on CI failure; no local E2E ran. Current-head remote CI remains pending until the correction is pushed and evaluated.
