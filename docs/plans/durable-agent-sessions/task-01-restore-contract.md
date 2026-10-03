---
id: "01-restore-contract"
title: "Define the restore contract"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 01: Define the restore contract

## Summary

Introduce unconditional restore policy with typed outcomes and conservative harness capabilities.

## In scope

- Make native-identity preservation and explicit replacement authorization the default contract. Add no runtime flag or hidden opt-in.
- Define restore reason codes, action authorization, incarnation/generation references, and optional adapter capabilities.
- Classify native-state evidence at the adapter boundary. Keep transport and unknown errors non-destructive.

## Out of scope

Snapshot storage, restore orchestration, and v1 transport advertisement.

## Acceptance

- Unknown and transport errors retain native identity. Missing native state only offers the explicit continuation action.
- Every caller uses the same restore policy. Unsupported native capabilities return typed outcomes rather than an undocumented replacement.
- Native resume/load negotiation remains compatible with older adapters.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/session_restore_policy_test.go`:

- `TestRestorePolicyPreservesNativeIdentity`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1`.
- `TestRestorePolicyRequiresExplicitContinuation`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.2`.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/agents/`
- `apps/backend/internal/agent/runtime/lifecycle/session_restore_policy_test.go` (new tests or extensions).

## Dependencies

None. Read the complete design package before implementation.

## Risks

- Generic ACP errors can resemble missing-state errors. Unknown evidence must remain blocked.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/agents/system-design/harness-session-continuity.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented in `session_restore_policy.go` and `session_restore_policy_test.go`. Typed native-state, transport, capability, and explicit-action outcomes preserve the committed native identity. Focused policy and restore integration tests pass.
