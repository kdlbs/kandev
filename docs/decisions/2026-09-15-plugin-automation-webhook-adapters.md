# ADR-2026-09-15-plugin-automation-webhook-adapters: Plugin-owned webhook adapters with host-bound automation authority

**Status:** accepted
**Date:** 2026-09-15
**Area:** protocol

## Context

Bitbucket repositories are supplied by a separately released plugin, but the core
automation endpoint accepts only a literal secret header. Bitbucket signs its
payload instead. The automation condition registry also has no plugin extension
point. Issue [#881](https://github.com/kdlbs/kandev/issues/881) proposed signed
verification and adapters; [#3264](https://github.com/kdlbs/kandev/issues/3264)
provides another external-event use case. Neither specifies this plugin contract.

The user selected specification work for plugin-provided conditions and adapters
following discussion of core HMAC support and relay alternatives. This decision
records that ownership boundary. The detailed
[requirements](../specs/plugins/requirements/automation-webhook-adapters.md) and
[system design](../specs/plugins/system-design/automation-webhook-adapters.md)
remain drafts for review; this ADR does not claim the capability is implemented or
that every proposed protocol detail has been accepted.

## Decision

Provider-specific verification, event interpretation, and condition metadata live
in plugins. Kandev owns the native automation editor, explicit workspace-scoped
bindings, secret custody, lifecycle checks, delivery admission, and execution.
The plugin system owns the contribution contract and its vertical specifications;
the existing automation system retains execution ownership.

External payloads and plugin responses cannot choose the destination workspace or
automation. Only a user-authorized binding provides that authority. Add an optional
typed adapter contract without changing generic HTTP callback semantics. Existing
GitHub and generic webhook behavior remains compatible.

The first host contract routes adapters by plugin and condition identity, without
an unused separate adapter key. Binding revisions include installation identity;
upgrades and reinstalls require user reconfiguration with a fresh secret while
ordinary restarts preserve the binding. Pending/unclaimed dispatch has a persisted
bounded retry budget, and cancellation settles admitted runs before receipt
removal. Scheduled triggers cannot coexist with plugin-event conditions.

The [delivery plan](../plans/automation-webhook-adapters/plan.md) records host
implementation and corrective verification; live provider acceptance remains a
separate integration check.

## Consequences

Providers can add conditions without adding provider-specific branches to core.
The host and first plugin must ship compatible contracts together. Registration,
configuration, delivery recovery, lifecycle revocation, and native UI support all
require host work; declaring a plugin webhook alone cannot provide this feature.
The host trusts installed plugin code to verify its provider's signatures, while
constraining the destination and admission policy independently.

## Alternatives Considered

- **Bitbucket-specific core authentication:** Small immediate change, but puts
  provider authentication in core while repository support lives in a plugin;
  does not supply plugin conditions in the editor.
- **External verification relay:** Can bridge the current authentication mismatch,
  but adds an operator-managed service and does not extend native conditions.
- **Generic HTTP callback calling an automation API:** Reuses ingress but conflates
  arbitrary callback responses with verified events and requires extra authority.
  A typed return to a pre-authorized binding keeps that authority narrower.
- **Replace all existing triggers:** Expands migration and compatibility risk
  without being necessary to enable Bitbucket or other plugin events.
