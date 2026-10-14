# ADR-2026-10-08-unconditional-interruption-continuation: Make safe interruption continuation normal recovery

**Status:** accepted
**Date:** 2026-10-08
**Area:** backend, protocol, frontend

## Context

A Cursor session ran for 49 minutes before a recognized HTTP/2 RetriableError.
Kandev refused original-prompt replay because the turn had already produced work.
The separate continuation path was unavailable because its installation toggle
was off; the saved failure recorded zero retry attempts. The user explicitly
requested that this recovery stop depending on a feature flag and become the
default behavior.

The [completed-tool decision](2026-10-05-hidden-completed-tool-continuation.md)
already defines bounded same-conversation recovery with an internal `continue`,
versioned provider evidence, and strict refusal after uncertain work. The
installation toggle is an additional availability condition, not a substitute
for those safety checks.

## Decision

Make supported interruption continuation normal recovery in prod, dev, and e2e,
including managed and standalone agentctl. Remove the live
`features.providerInterruptionContinuation` /
`KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION` gate throughout the startup,
evidence, admission, dispatch, and feature-state contracts. Retire both identities
in the append-only registry. Preserve old overrides as inert rows; do not replace
the flag with another opt-in or interpret stale false values as current policy.

Retain the original-prompt replay fence, versioned native restoration support,
complete foreground outcome evidence, five-attempt schedule, internal hidden
`continue`, cancellation, and human-priority rules. Pending or cancelled tools
remain manual. Older helpers with missing evidence remain unsupported for
automatic continuation. Graduation does not replay past failures on upgrade.

## Consequences

Eligible supported interruptions recover without operator setup. Feature Toggles
and feature-state responses no longer expose this setting. Existing historical
failure messages remain readable, and the change requires delivery of updated
backend and agentctl components rather than an installation override.

The runtime toggle can no longer disable this recovery policy. Provider capability
and prompt-safety conditions still control each attempt. This decision changes
availability only and does not promise exactly-once model actions or automatic
recovery for the interrupted shell tool shown in the reported session.

## Alternatives Considered

- Promote profile defaults to true while retaining a kill switch. Rejected because
  the user requested removal of the feature-flag dependency; stale overrides
  would continue to disable ordinary recovery.
- Enable only the affected installation. Rejected because the request establishes
  shipped behavior rather than an operator workaround.
- Replay the original prompt after output or tools. Rejected because it can repeat
  instructions whose actions already completed.

## Related contracts

- [Interruption continuation requirements](../specs/platform/requirements/provider-interruption-continuation.md)
- [Interruption continuation design](../specs/platform/system-design/provider-interruption-continuation.md)
- [Graduation delivery package](../plans/provider-interruption-continuation-graduation/plan.md)
