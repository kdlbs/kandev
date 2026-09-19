# Product principles

**Status:** Proposed baseline for product review.

These principles are synthesized from the current system specifications, public
product boundary, and ADRs. They describe how Kandev should behave across
systems. They are not a replacement for system requirements.

## Human control at irreversible boundaries

Agents can prepare and execute work, but a human remains able to inspect the
result before merge, release, deployment, deletion, or another irreversible
operation. Product flows should make the decision point visible and preserve
the evidence needed to make it.

Evidence: [sessions and review](../../public/sessions-and-review.md),
[security and trust](../../public/security.md), and the task system.

## Backend authority, client recovery

Durable state has one backend owner. Browser, desktop, API, and WebSocket
surfaces render and mutate that state through explicit contracts. A client must
recover from missed, duplicated, stale, or reordered events rather than making
local state authoritative.

Evidence: [architecture](../../public/architecture.md) and the platform,
task, and UI system specifications.

## Explicit trust boundaries

Every boundary must state what is trusted, what is exposed, and who owns the
decision. Repository content, provider text, agent output, URLs, archives,
paths, and command arguments are untrusted inputs. Permissions are validated
server-side and credentials are scoped to the smallest practical environment.

Evidence: [security and trust](../../public/security.md), the auth, agent,
executor, integration, and plugin systems.

## Work has durable context

A task is more than a prompt. It carries a desired outcome, workspace and
repository context, workflow position, agent sessions, execution state, and
review evidence. Restarting or switching surfaces should preserve that context
without silently changing its owner.

Evidence: [tasks and workflows](../../public/tasks-and-workflows.md), the task
and workspace systems, and the runtime recovery decisions.

## Ownership is explicit and non-duplicated

Each product contract has one owning system. Other systems reference it rather
than copying it. Product context explains relationships, system requirements
define observable behavior, and system designs define technical boundaries.

Evidence: [specification guide](../guide/README.md) and the system-oriented
catalog.

## Provider and executor neutrality

Kandev should coordinate different agent providers and execution environments
without making provider-specific assumptions part of the core task model.
Provider and executor differences belong behind explicit profiles, adapters,
capabilities, and status boundaries.

Evidence: the agent, executor, integration, and platform systems, plus
[architecture](../../public/architecture.md).

## Fail closed and recover safely

Invalid configuration, missing capability, failed authentication, provider
errors, and unsafe requests should fail before side effects when possible. When
work is interrupted, startup reconciliation and durable cleanup should preserve
user work and expose actionable state instead of silently discarding it.

Evidence: [feature status](../../public/feature-status.md), [security and
trust](../../public/security.md), and the runtime cleanup and recovery ADRs.

## Make progress observable

Users should be able to understand what Kandev is doing, what the agent did,
what remains, and why a task is blocked. Status, logs, changes, review evidence,
and diagnostics should use stable identities and clear ownership.

Evidence: the UI, system-page, platform, task, and session specifications.

## Extend through contracts

Integrations and plugins should extend Kandev through reviewed host contracts.
They should not reach around authentication, task ownership, persistence, or
security boundaries through undocumented shortcuts.

Evidence: the integration and plugin systems and the plugin host ADRs.

## Be honest about capability status

Documentation must distinguish supported, dependency-bound, limited,
experimental, in-progress, and internal behavior. A schema, hidden route,
mock-only test, or feature flag is not by itself a supported product promise.

Evidence: [feature status](../../public/feature-status.md).

## Preserve capability parity across surfaces

Responsive web, desktop, CLI, API, and MCP surfaces may have different
interaction patterns, but the product must make intentional capability limits
visible. Mobile behavior is a native interaction surface, not only a smaller
desktop layout. User-facing UI copy is localized through the established
translation system.

## Open product questions

- Which principles should become formally numbered and referenced by future
  requirements or ADRs?
- What measurable evidence should determine when a principle is not being met?
- Which future product decisions could intentionally override one of these
  principles, and where should that exception be recorded?
