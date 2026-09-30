# ADR-2026-09-25-plugin-coordination-platform: Extensible coordination through generic Host contracts

**Status:** accepted
**Date:** 2026-09-25
**Area:** backend, frontend, protocol, workflow

## Context

The user approved a planning package for user-defined coordinator plugins after
reviewing Corey's workspace-orchestration fork. That fork demonstrates roles,
workspace instances, chat, durable callbacks, proposals, task management, criteria,
tracker writes, schedules, and outcomes. Copying its product runtime into Kandev
would make each different coordination policy a core feature change.

The 2026-08-31 generic Host ADR already proposes much of the required boundary.
The capability approval substrate is implemented, but most exact Host operations,
managed-agent restriction, and shared workspace chat remain delivery work.
This decision accepts the ownership boundary and scoped extensions below. It does
not claim the full older proposal is accepted or implemented.

## Decision

Kandev owns generic execution, durable input admission, canonical task observations,
shared commands, task claims, completion gates, native interaction consent, and
reusable chat components. Plugins own roles, policy, memory, proposals, trigger
selection, reconciliation, reports, and product composition.

Build additive capability-gated exact APIs and reuse the shipped approval ledger.
Retain v1 wire shapes and capability declarations; existing interactions are a
scoped exception to v1 behavior: the Host denies legacy v1 permission and
clarification response methods because they cannot carry a native human response
receipt or the observed interaction revision. Use the older ADR's exact operation
names where applicable. Keep
Add durable enqueue separately from immediate dispatch: a busy conversation accepts
queued input only through the explicit queue API. Accepted input is not completed
agent work. Managed conversation retention is opt-in and does not change v1 cleanup.

Task claims and completion gates are task-owned shared invariants. All callers pass
through them. Restricted agent policy is runtime-owned, enforceable by adapters,
and fail-closed for unsupported providers. Automation delivers to a generic managed
conversation destination. Human consent cannot be forged by a plugin tool argument.

Deliver one reference plugin and a second independently packaged policy variant in
dedicated repositories. Host test fixtures are test support only. Neither a
coordinator-specific core framework nor a universal policy DSL is required.

The [requirements and designs](../plans/plugin-coordinator-platform/plan.md) define
the delivery scope. The older ADR's automatic merge, provider mutation expansion,
terminal cleanup framework, and broader maintenance campaigns are not adopted here.
The immutable human-reserved capability restrictions continue to apply.

## Consequences

Users can customize coordination without maintaining a host fork. Host invariants
remain shared with ordinary task workflows. Independent consumers test whether the
extension surface actually supports different strategies.

This requires durable command receipts, lifecycle reconciliation, new public API
contracts, and adapter-specific tool enforcement. Native plugins remain trusted
code; managed-agent restrictions are not an OS sandbox for installed plugins.
Uncertain model or provider outcomes require reconciliation, not blind replay.

The plan records the accepted delivery sequence. Each work order updates the public
SDKs and authoring docs with the operations it ships. Implementation is underway
under the user's explicit request to implement that package.

## Alternatives Considered

- Merge the fork's product runtime into core: rejected because policy and product
  state would become host release concerns and restrict independent variants.
- Package only prompts around current APIs: rejected because queued input, runtime
  restrictions, canonical observations, and guarded controls are incomplete.
- Give plugins private REST or database access: rejected because that bypasses
  durable compatibility, authorization, and shared domain invariants.
- Create a universal coordinator framework or DSL: deferred because two public-API
  consumers can establish the needed abstractions with less permanent machinery.

## Evidence and related decisions

- [Fork snapshot](https://github.com/Corey-Fogg/kandev/tree/8017d2a32b7e72de3e0b14fff48a345d87b9891d)
- [Generic Host proposal](2026-08-31-generic-plugin-host-boundary.md)
- [Plugin tools through Kandev MCP](2026-08-11-plugin-tools-through-kandev-mcp.md)
- [Capability approval design](../specs/plugins/system-design/capability-approval.md)
- [Delivery plan](../plans/plugin-coordinator-platform/plan.md)
