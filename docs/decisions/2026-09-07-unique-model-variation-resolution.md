# ADR-2026-09-07: Resolve Only One Advertised Model Variation

**Status:** rejected (superseded by `2026-09-06-exact-profile-model-identity`)
**Date:** 2026-09-07
**Area:** backend, frontend, protocol
**Superseded by:** `2026-09-06-exact-profile-model-identity`

> This proposed policy is rejected. An inferred variation is not the exact
> model identity selected by the profile, so it cannot authorize inference.
> The accepted exact-profile policy governs model selection instead.

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

## Rejection

This proposal was not adopted. It would have inferred a bracketed advertised
model from a bare profile model and treated that inferred ID as an authorized
selection. That conflicts with the accepted exact-profile boundary: an
inferred ID is a different runtime identity from the configured model.

No variation-matching algorithm, inferred-selection outcome, warning reason,
or launch, reset, or workspace-rebind behavior was adopted from this proposal.
The accepted policy is recorded exclusively in
[Enforce Exact Profile Model Identity](2026-09-06-exact-profile-model-identity.md).

## Consequences of rejection

- The historical `opus` to `opus[1m]` example does not authorize inference.
- Provider-specific variation labels and catalog order do not establish a
  substitute model identity.
- Any future variation policy requires a new accepted decision that is
  compatible with exact-profile identity.

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
