---
status: draft
system: platform
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Platform system

## Purpose

The platform system owns cross-cutting runtime services, configuration,
observability, notifications, localization, lifecycle safety, and shared
operational guarantees.

## Ownership

This system owns startup and shutdown contracts, process and port-independent
runtime safety, configuration precedence, diagnostics, notifications,
localization, feature toggles, health, and shared session recovery services.

## Exclusions

- Executor-specific runtime environments belong to the [executor
  system](../executors/README.md).
- Agent identity belongs to the [agent system](../agents/README.md).
- Desktop shell behavior belongs to the [desktop system](../desktop/README.md).

## Specification map

### Requirements



- [Agent process exit and stderr drain](requirements/agent-process-exit-drain.md)
- [Agentctl instance stop idempotency](requirements/agentctl-instance-stop.md)
- [Agent Runtime Availability](requirements/agent-runtime-availability.md)
- [Background Work Liveness](requirements/background-work-liveness.md)
- [Bounded Task Status Delivery](requirements/bounded-task-status-delivery.md)
- [Shared Cron Loop Safety When Office Is Disabled](requirements/cron-office-disabled-safety.md)
- [Environment-specific browser tab title prefixes](requirements/dev-preview-title-prefixes.md)
- [Browser console retention](requirements/browser-console-retention.md)
- [Diagnostic logging](requirements/diagnostic-logging.md)
- [Duration-aware E2E sharding and CI reliability](requirements/e2e-duration-aware-sharding.md)
- [Expected runtime log severity](requirements/expected-runtime-log-severity.md)
- [Feature Toggles](requirements/feature-toggles.md)
- [Git Credential Lease Reissue](requirements/git-credential-lease-reissue.md)
- [Git Subprocess Admission](requirements/git-subprocess-admission.md)
- [Go dev launcher and minimal Node surface](requirements/go-dev-launcher.md)
- [Health Endpoint — Surface the Running Version](requirements/health-endpoint-version.md)
- [Watcher And Task Fallback Localization](requirements/i18n-audit-watcher-copy.md)
- [Additional I18n Audit Gaps](requirements/i18n-second-audit-gaps.md)
- [Internationalization (i18n)](requirements/i18n.md)
- [LSP File Intelligence](requirements/lsp-file-intelligence.md)
- [Session MCP Attachment Observability](requirements/mcp-session-observability.md)
- [Mid-Turn Steering](requirements/mid-turn-steering.md)
- [Semantic Notifications](requirements/notifications.md)
- [Provider Error Recovery](requirements/provider-error-recovery.md)
- [Session Config Reconciliation Across Agent Types](requirements/session-config-cross-agent-reconcile.md)
- [Session subscription recovery](requirements/session-subscription-recovery.md)
- [Setup and Launch Timeout](requirements/setup-launch-timeout.md)
- [Quiet benign teardown log noise on shutdown](requirements/shutdown-log-noise.md)
- [Do not surface backend-shutdown turn aborts as agent failures](requirements/shutdown-turn-failure-suppression.md)
- [Startup Configuration Parity](requirements/startup-configuration-parity.md)
- [Prevent Host Sleep During Active Tasks](requirements/task-sleep-inhibition.md)
- [Traditional Chinese locales (Taiwan and Hong Kong)](requirements/traditional-chinese-locales.md)
- [Workspace Git Status](requirements/workspace-git-status.md)

### System design



- [Agent process exit and stderr drain](system-design/agent-process-exit-drain.md)
- [Agentctl instance stop idempotency](system-design/agentctl-instance-stop.md)
- [Browser console retention](system-design/browser-console-retention.md)
- [Bounded Task Status Delivery](system-design/bounded-task-status-delivery.md)
- [Diagnostic logging System Design Part 1](system-design/diagnostic-logging-01.md)
- [Diagnostic logging System Design Part 2](system-design/diagnostic-logging-02.md)
- [Health Endpoint — Surface the Running Version](system-design/health-endpoint-version.md)
- [Internationalization (i18n)](system-design/i18n.md)
- [LSP File Intelligence System Design Part 1](system-design/lsp-file-intelligence-01.md)
- [LSP File Intelligence System Design Part 2](system-design/lsp-file-intelligence-02.md)
- [Session MCP Attachment Observability](system-design/mcp-session-observability.md)
- [Provider Error Recovery](system-design/provider-error-recovery.md)
- [Workspace Git Status](system-design/workspace-git-status.md)
- [Go dev launcher and startup version](system-design/go-dev-launcher.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Agents](../agents/README.md): consumes shared runtime services.
- [Executors](../executors/README.md): owns execution-environment details.
- [Desktop](../desktop/README.md): embeds platform startup and shutdown.
