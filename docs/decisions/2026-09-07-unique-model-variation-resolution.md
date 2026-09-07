# ADR-2026-09-07: Resolve Only One Advertised Model Variation

**Status:** proposed
**Date:** 2026-09-07
**Area:** backend, frontend, protocol
**Amends:** `2026-08-15-executor-authoritative-model-selection`

## Context

An agent CLI can replace a bare model ID with bracketed variations. A profile
can keep `opus` while the executor later advertises only `opus[1m]`.

The current executor-authoritative policy treats the saved ID as absent. It
uses an explicit fallback when available. Otherwise, it continues with the
provider current or default model and records a warning. This result can move a
session to an unrelated model even when the catalog has one clear variation of
the requested model.

Variation text is provider-owned. Kandev cannot safely rank `270k`, `1m`,
`fast`, or future labels. Catalog order and the current-model marker do not
express user intent.

## Decision

Kandev recognizes a model variation only by a narrow bracketed ID shape. A bare
requested ID `x` matches an advertised ID `x[v]` when `v` is non-empty and
contains no bracket. Matching is exact and case-sensitive. Kandev treats `v` as
opaque text.

For profiles with `auto_fallback = false`, the executor-authoritative resolution
order is:

1. Exact requested model.
2. Advertised explicit fallback from the profile.
3. One distinct advertised variation of the requested bare model.
4. Provider current or default model, with no speculative model-selection call.

Kandev applies step 3 only when exactly one distinct candidate exists. Zero or
multiple candidates produce no inferred selection. An unadvertised explicit
fallback does not block step 3.

A requested ID that contains a bracket is an exact ID only. Kandev does not
infer another variation from it.

Profiles with `auto_fallback = true` retain the legacy path when the requested
model is absent. Kandev ignores the configured explicit fallback, does not
infer a variation, and continues with the provider current or default model.
Apply errors for an advertised model remain best-effort in that mode.

The lifecycle stores `unique_variation` as the decision outcome and
`unique_variation_applied` as the warning reason. The effective model contains
the advertised variation. The explicit fallback field keeps its existing
meaning and stays empty when Kandev infers a variation.

The saved profile model does not change. The rule runs for initial launch,
context reset, and workspace rebind. The executor catalog remains authoritative.
Any host-catalog result remains an advisory.

## Consequences

- A profile with `opus` follows `opus[1m]` when it is the only advertised
  variation.
- A catalog with `opus[270k]` and `opus[1m, fast]` remains ambiguous and uses
  the existing provider-default path.
- An explicit advertised fallback remains stronger than inference.
- Kandev never sends a model ID that the executor did not advertise.
- Users receive a durable warning because the effective model differs from the
  saved request.
- Provider-specific ranking logic does not enter the lifecycle policy.
- Backend and frontend helpers need mirrored contract tests. The backend result
  remains authoritative when their input catalogs differ.

## Alternatives Considered

1. **Always choose the first variation.** Rejected because catalog order is not
   a user preference or a stable provider contract.
2. **Prefer the provider current model.** Rejected because current state can
   reflect a default or an earlier session, not the saved profile intent.
3. **Rank known labels such as context size or fast mode.** Rejected because
   labels are provider-owned and can change without a Kandev release.
4. **Rewrite the saved profile to the inferred model.** Rejected because one
   executor observation must not change behavior for every executor.
5. **Infer from an already bracketed request.** Rejected because it can replace
   one explicit variation with another without user intent.
