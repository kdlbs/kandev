---
id: "01-load-failure"
title: "Preserve identity after load failure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.1
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 01: Preserve identity after load failure

## Summary

Classify native load failures before fallback. Preserve the original token on
inconclusive errors, including the exact serialized deadline from the incident.

## In scope

- Add barrier-free lifecycle regressions using the existing fake agentctl server.
- Permit existing fallback only for positively identified supported cases.
- Keep error causes available to the recovery layer without exposing raw secrets.

## Out of scope

Provider internals, new queue policy, schema changes, and live task repair.

## Acceptance

- A nested deadline, internal error, or authentication failure never creates a new conversation.
- Confirmed unsupported and missing-session fixtures retain their current behavior.
- A healthy retry receives the original stored conversation identity.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test(CreateOrLoadSession|IsTransportDeadErr|SessionLoadFailure)' -count=1)
```

Run new regressions before production changes and record the expected failure.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_load_failure_test.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go (existing fixture reference)`

## Dependencies

None.

## Risks

Preserve supported provider behavior and avoid lock inversion. Late cleanup
must not mutate the current attempt. Use deterministic barriers, not sleeps.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/session-recovery-failures.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- [Plan evidence and regression map](plan.md).
- Existing resume tests, cancellation helpers, and contribution recovery card.

## Results

Implemented positive fallback classification in the lifecycle session loader.
Serialized deadlines and internal, authentication, and unknown transport
errors now preserve the saved provider conversation identity. Confirmed
method-not-found, unsupported-capability, and unknown-session responses retain
their existing `session/new` fallback.

The lifecycle regression file covers both sides of the classification and the
focused race test passes.
