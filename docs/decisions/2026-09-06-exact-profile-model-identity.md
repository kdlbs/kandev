# ADR-2026-09-06-exact-profile-model-identity: Enforce Exact Profile Model Identity

**Status:** accepted
**Date:** 2026-09-06
**Area:** backend, frontend, protocol, persistence, workflow

## Context

An executor catalog remains authoritative for whether a model can be selected,
but allowing its default after an exact profile's model is absent changes the
user's chosen runtime identity and cost policy before the first prompt.

## Decision

A profile with a non-empty model, `auto_fallback = false`, and no
`fallback_model` is exact. Its configured model must be advertised and applied
before inference. If that cannot be attested, the session fails before a
prompt, tool call, or agent output.

Kandev never sends `SetModel` for an unadvertised model. An advertised explicit
fallback is the only authorized alternate model when auto fallback is off.
`auto_fallback = true` explicitly authorizes continuation on the executor
default and keeps its durable warning behavior. This same decision applies on
initial launch, context reset, and fresh workspace rebind.

## Consequences

Exact profiles can fail when an executor, account, or pinned runtime does not
advertise their configured model. The failure records sanitized requested,
effective-when-known, and stable mismatch-reason evidence. Existing workflow
profile precedence and intentional same-session runtime overrides remain
unchanged.

## Alternatives Considered

1. Continue on the executor default for every catalog mismatch. Rejected
   because it silently substitutes the chosen model identity.
2. Send an unadvertised model selection request. Rejected because the executor
   catalog is still the availability authority.
3. Rewrite the saved profile from an executor observation. Rejected because a
   transient executor/account state must not mutate the user's profile.
