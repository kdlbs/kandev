---
created: 2026-11-19
status: done
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
legacy_specs: []
---

# Implementation plan: Devin free-model rate limit classification

## Overview

`devin-acp` has no provider rule. Its free-plan notice ("Reached free model
rate limit. ... Your limit will reset in 8 minutes (at 12:01 UTC)") therefore
classifies as the phase default: `agent_runtime_error`, low confidence, no
fallback. Dynamic routing keeps sending prompts to the throttled candidate
instead of suspending it until the stated reset and selecting another one.
This package adds a scoped `devin-acp` rate-limit rule and derives the reset
from the notice, as defined by `AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.13` and
the "Devin free-model rate limit" section of the system design.

## Scope

In scope: the `devin-acp` rule, the clock-with-duration reset parser, and
classifier tests (positive variants, day rollover, relative-duration
disambiguation, prose and fence negatives, other providers, line-scoped reset).

Out of scope: rule order or flags of other providers, routing policy, circuit
handling, and any frontend change.

## Work orders

- [x] [Task 01: Classify the Devin free-model rate limit](task-01-classify-free-model-rate-limit.md)

One work order: one rule plus its reset parser, with one verification boundary.

## Verification strategy

Backend unit tests in `apps/backend/internal/agent/runtime/routingerr`. No
end-to-end test: the outcome is a classification that existing routing and
recovery consumers already handle for other providers.

## Risks

- The notice wording can change. An unmatched variant falls back to the
  current phase default, so the failure mode is today's behavior.
- A wrong day choice could park a candidate for a day. The relative duration
  disambiguates the day, and an elapsed hint only shortens the suspension.

## References

- [Requirement](../../specs/agents/requirements/provider-error-signatures.md)
- [System design](../../specs/agents/system-design/provider-error-signatures.md)
