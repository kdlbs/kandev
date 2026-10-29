---
status: draft
system: agents
created: 2026-10-28
owners:
  - kandev
---

# Kandev integration plugin for OpenCode v2

## Overview

Kandev supplies one integration plugin when it launches a supported OpenCode v2
runtime. Users do not install or start this plugin separately. The first behavior
reports native shell outcomes that OpenCode does not publish through ACP.

Agents owns this contract because it defines provider integration and capability
ownership. Tasks stores its observations. Platform transports them. The existing
background-work viewer shows their results.

The integration is an OpenCode server plugin. It is separate from marketplace
plugins that extend the Kandev host. Future behaviors extend this integration
through explicit modules with their own contracts.

## Terms

- **Integration plugin:** the single Kandev-supplied OpenCode server extension.
- **Behavior:** an independently enabled capability inside that extension.
- **Launch lease:** the integration identity for one Kandev-owned agent process.
- **Claim:** Kandev's association between a confirmed background launch and a
  native session, shell, and origin turn.
- **Supported runtime:** an OpenCode v2 runtime with recorded plugin compatibility
  evidence. A major version alone does not establish support.

## Requirements

### REQ-AGENTS-OPENCODE-PLUGIN-001: Automatic launch integration

**Intent:** Supply the integration with the runtime that owns the conversation.

- **AC-AGENTS-OPENCODE-PLUGIN-001.1:** When the integration is enabled, Kandev shall supply and load it for supported OpenCode v2 task sessions over ACP.
- **AC-AGENTS-OPENCODE-PLUGIN-001.2:** When Kandev supplies the integration, users shall not need a separate plugin installation, npm command, or start action.
- **AC-AGENTS-OPENCODE-PLUGIN-001.3:** When a supported session starts on another executor, that executor shall supply the same reviewed integration artifact and launch contract.
- **AC-AGENTS-OPENCODE-PLUGIN-001.4:** When Kandev supplies the integration, existing user plugins, configuration precedence, credentials, and MCP configuration shall retain their meanings.
- **AC-AGENTS-OPENCODE-PLUGIN-001.5:** When a session starts again, Kandev shall supply one integration instance for its new process. Repeated preparation shall not duplicate plugin entries.
- **AC-AGENTS-OPENCODE-PLUGIN-001.6:** When Kandev adopts a retained process, it shall retain that process's integration ownership. It shall not install another plugin into the live process.
- **AC-AGENTS-OPENCODE-PLUGIN-001.7:** When v1, unsupported runtimes, probes, utilities, or CLI passthrough start, Kandev shall preserve their current launch without this integration.

### REQ-AGENTS-OPENCODE-PLUGIN-002: Explicit behavior modules

**Intent:** Add future Kandev behaviors without a second integration lifecycle.

- **AC-AGENTS-OPENCODE-PLUGIN-002.1:** When Kandev adds a compatible behavior, it shall reuse plugin installation, ownership, authentication, delivery, and cleanup.
- **AC-AGENTS-OPENCODE-PLUGIN-002.2:** When one behavior is disabled or unsupported, the integration shall not activate it. Independently supported behaviors shall remain available.
- **AC-AGENTS-OPENCODE-PLUGIN-002.3:** When a behavior or its contract version is unknown, the receiver shall reject its effects without granting another capability.
- **AC-AGENTS-OPENCODE-PLUGIN-002.4:** When the initial plugin runs, it shall expose only the approved shell-outcomes behavior. Extension support shall not expose arbitrary commands, scripts, prompts, or host mutations.
- **AC-AGENTS-OPENCODE-PLUGIN-002.5:** When a behavior reports an error, ordinary ACP conversation shall remain available. The integration shall report the affected capability as unavailable.

### REQ-AGENTS-OPENCODE-PLUGIN-003: Owned and reliable observations

**Intent:** Apply genuine observations to the original job without changing turn ownership.

- **AC-AGENTS-OPENCODE-PLUGIN-003.1:** When an observation arrives, Kandev shall accept its effects only for the current launch lease and an owned native-session claim.
- **AC-AGENTS-OPENCODE-PLUGIN-003.2:** When a callback has a foreign execution, stale lease, removed claim, or superseded native session, it shall have no effects.
- **AC-AGENTS-OPENCODE-PLUGIN-003.3:** When delivery retries an accepted observation, Kandev shall retain one logical result. A delayed observation shall not reverse a terminal outcome.
- **AC-AGENTS-OPENCODE-PLUGIN-003.4:** When a sender receives an acceptance acknowledgement, the observation shall have crossed Kandev's durable delivery boundary.
- **AC-AGENTS-OPENCODE-PLUGIN-003.5:** When a result precedes its launch association, bounded correlation shall preserve the result or report uncertainty. It shall not invent an association.
- **AC-AGENTS-OPENCODE-PLUGIN-003.6:** When a stream gap, capacity limit, or runtime loss removes evidence, Kandev shall preserve known history and leave unproved outcomes unknown.
- **AC-AGENTS-OPENCODE-PLUGIN-003.7:** When the plugin reports a job outcome, it shall preserve conversation turns, workflow placement, tasks, and prompt admission.
- **AC-AGENTS-OPENCODE-PLUGIN-003.8:** When diagnostics describe the integration, they shall exclude callback credentials, prompts, command output, and private environment values.

### REQ-AGENTS-OPENCODE-PLUGIN-004: Controlled activation and compatibility

**Intent:** Activate reviewed provider support without changing runtime selection.

- **AC-AGENTS-OPENCODE-PLUGIN-004.1:** When shipped profiles use their initial defaults, the integration shall remain disabled. An explicit operator override shall enable automatic integration on the next supported launch.
- **AC-AGENTS-OPENCODE-PLUGIN-004.2:** When the integration is disabled, Kandev shall not supply its files, listeners, credentials, subscriptions, or observations for that launch.
- **AC-AGENTS-OPENCODE-PLUGIN-004.3:** When runtime or plugin compatibility cannot be established, Kandev shall retain ordinary ACP behavior and the existing unknown-job fallback.
- **AC-AGENTS-OPENCODE-PLUGIN-004.4:** When the selected runtime is incompatible, the integration shall not migrate, upgrade, replace, or downgrade OpenCode automatically.
- **AC-AGENTS-OPENCODE-PLUGIN-004.5:** When shared background-work support is disabled, the shell-outcomes behavior shall not claim jobs or publish workload observations.
- **AC-AGENTS-OPENCODE-PLUGIN-004.6:** When equivalent upstream capability becomes available, Kandev shall select one proven observation source for each behavior. It shall not publish duplicate results from two sources.

## Initial behavior and related requirements

[Background-work requirements](background-work.md) own shell outcomes, history,
desktop and phone results, and truthful unknown activity. In particular,
AC-AGENTS-BACKGROUND-WORK-007.6 through .9 define this module's initial outcome.
This document does not duplicate those criteria.

## Out of scope

Automatic assistant-response delivery, active-background liveness, process
controls, live output, transcript recovery, historical backfill, and OpenCode v1
plugins remain outside this package. The plugin does not become a general remote
command service. Future behaviors require explicit requirements and evidence.

- [System design](../system-design/opencode-kandev-plugin.md)
- [Implementation package](../../../plans/opencode-kandev-plugin/plan.md)
