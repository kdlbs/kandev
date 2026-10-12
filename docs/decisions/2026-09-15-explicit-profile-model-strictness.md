# ADR-2026-09-15-explicit-profile-model-strictness: Make Exact Model Selection a Profile Opt-in

**Status:** accepted
**Date:** 2026-09-15
**Area:** backend, frontend, persistence, workflow

## Context

PR #3473 makes existing profiles strict when automatic fallback is false and
no explicit fallback is saved. These values previously permitted executor-default
continuation in catalog mismatch cases. Existing stored values do not prove
that a user selected strict behavior. Upgrading must not introduce that failure.

## Decision

Add a per-profile `require_exact_model` setting, default false for existing and
new profiles. Strictness takes priority over dormant fallback choices. Turning
it off restores those saved choices without rewriting them. Require a concrete
model when strictness is enabled. Do not add a global or workspace override.

Compatible profiles retain the pre-PR selection/error behavior, including the
single advertised variation rule. Strict profiles never infer variations and
fail before inference when the requested model cannot be advertised and applied.
Both paths retain executor authority, unchanged saved model IDs, and durable
fallback warnings. No policy sends an unadvertised model.

This decision supersedes
[implicit exact-profile identity](2026-09-06-exact-profile-model-identity.md).
It replaces the blanket variation rejection recorded in
[the variation decision](2026-09-07-unique-model-variation-resolution.md) with a
compatible-only rule. The implementation is tracked in the
[PR package](../plans/exact-profile-model-identity/plan.md).

## Consequences

Upgrades preserve working mismatch launches without enabling broader automatic
fallback. Profiles can enforce exactness independently across executors. The
additional field must traverse storage, APIs, profile copies, runtime resolution,
and all editors. Existing fallback values remain dormant while strictness is on.
Office post-start provider routing remains separately workspace-owned.

## Alternatives Considered

- Infer strictness from existing false/empty fields: changes existing launch
  behavior and cannot distinguish an explicit decision from a default.
- Set automatic fallback true on every old profile: ignores explicit fallback
  models and tolerates errors that previously stopped the launch.
- Replace current fields with an enum: adds migration scope and risks losing
  existing fallback/error distinctions for this compatibility fix.
- Add global and profile settings: creates inheritance/precedence rules and
  broad effects unrelated to an individual profile's identity policy.
