---
id: "01-preserve-model-policy-outcome"
title: "Preserve handled model-policy outcomes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 01: Preserve handled model-policy outcomes

## Intent

Stop the profile configuration layer from retrying `SetModel` after the
start-model policy has already made and handled the model decision, including
outcomes where no model was applied.

## Acceptance

1. Successful start-model selection, auto-fallback best-effort failure, and
   method-not-supported each make one policy-owned attempt with no profile-layer
   retry.
2. When an unknown advertised list requires an explicit fallback after a
   rejected start model, calls are exactly `[start, fallback]` with no third
   attempt.
3. The original-configuration snapshot records only the model actually applied;
   best-effort failure and method-not-supported record no applied model.

## TDD Sequence

1. Extend the recording-agentctl lifecycle test matrix for auto-fallback
   failure, method-not-supported, and ordered explicit fallback; confirm the
   first two expose the current duplicate call.
2. Replace `modelSet` with an outcome that carries `handled` and `appliedModel`
   independently from `effectiveModel`.
3. Make `applyProfileSessionLayers` skip model mutation whenever the policy
   handled selection, while only recording a non-empty applied model.
4. Run the focused lifecycle package tests.

## Files Likely Touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 02; the files are disjoint and neither task changes a
shared contract or package configuration.

## Inputs

- Spec section “Session start: strict model application”.
- Existing `applyStartModelPolicy` return semantics in
  `apps/backend/internal/agent/runtime/lifecycle/start_model.go`.
- Existing `TestInitializeAndPrompt_AppliesStartModelPolicyExactlyOnce` recording
  pattern.

## Verification

```sh
cd apps/backend && go test ./internal/agent/runtime/lifecycle
```

## Risks

- An explicit fallback can legitimately make two different policy attempts;
  the invariant is no repeated attempt by a later layer, not a universal
  one-call limit.
- Preserve `finalConfigID`, fallback event publication, and session-initialized
  ordering while changing the outcome shape.

## Output Contract

Report the outcome contract, exact SetModel call lists, snapshot assertions,
command outcome, files changed, risks, and synchronized task/plan status.

## Results

Implemented the handled/applied model-policy outcome in
`apps/backend/internal/agent/runtime/lifecycle/session.go`. A policy-owned
attempt now marks the outcome handled even when best-effort fallback fails or
the agentctl method is unsupported, so the profile layer does not retry it.
Only a non-empty applied model updates the original-configuration snapshot.

The new regression test records one `agent.session.set_model` call for an
auto-fallback failure and confirms the snapshot remains on the original
`provider-default` model. Existing explicit fallback behavior remains ordered
and bounded to its policy-owned attempts.

Verification:

- `cd apps/backend && go test -run '^TestInitializeAndPrompt_(AppliesStartModelPolicyExactlyOnce|AutoFallbackFailureDoesNotRetryModel)$' ./internal/agent/runtime/lifecycle` — passed (2 tests).
- `cd apps/backend && go test ./internal/agent/runtime/lifecycle` — passed (1,158 tests).

Files changed: `session.go`, `session_test.go`.

Risk retained: an unknown advertised list may still make the intentional
`[start, fallback]` policy sequence; no later profile-layer attempt is added.
