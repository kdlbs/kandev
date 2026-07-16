---
title: "Extending Kandev"
description: "Add an agent, executor, integration, workflow capability, MCP tool, or settings surface using established extension points."
---

# Extending Kandev

Kandev has extension seams for agents, executors, integrations, workflow behavior, MCP tools, plugins, and UI settings. An extension is complete only when discovery/configuration, runtime behavior, failure handling, tests, and public documentation agree.

## Add an agent CLI

Agent definitions and protocol behavior live under `apps/backend/internal/agent/agents/` and register through `internal/agent/registry`. An integration describes install/discovery, ACP launch, optional passthrough, login, models/modes/capabilities, permission behavior, and MCP materialization.

Follow [Adding a new agent CLI](add-agent-cli.md). Test command construction, missing installation, login requirements, model/mode updates, permissions, resume, and the selected passthrough MCP strategy.

## Add an executor

Executor types are shared in `internal/agent/executor`; lifecycle implementations and registration live around `internal/agent/runtime/lifecycle`. Runtime-specific packages handle Docker, SSH, Sprites, worktrees, or host execution.

A production executor needs:

- profile models and secure credential fields;
- environment create/prepare/stop/cleanup behavior;
- agentctl delivery and control-channel connectivity;
- repository and multi-repository materialization;
- terminal, Git, files, ports, and metrics as supported;
- durable runtime IDs/status/errors for recovery;
- settings UI and validation;
- lifecycle, failure, and cleanup tests.

Do not advertise an executor before remote architecture, restart, cancellation, partial-create cleanup, and credential boundaries are documented.

## Add an integration

Provider packages such as `internal/github`, `gitlab`, `jira`, `linear`, `sentry`, and `slack` demonstrate the pattern: workspace-scoped config, secret storage, connection testing, provider client, service, handlers, optional watches/events, and settings UI.

Keep provider payloads behind a domain adapter. Validate server URLs and token types, redact errors, handle pagination/rate limits, and treat external text as untrusted agent input. A polling or webhook integration also needs deduplication and a visible health/run state.

## Add workflow or automation behavior

Workflow models, service, engine, adapters, and events live under `internal/workflow/`. Workspace automations live under `internal/automation/`. Preserve the distinction between task-local transitions and rules that create new work.

New events/actions need durable semantics, import/export representation when applicable, cycle/duplicate protection, UI editing, and tests for old workflow definitions.

## Add an MCP tool

Task/config MCP handlers and server registration live under `internal/mcp/handlers` and `internal/mcp/server`. Define a strict schema, enforce caller/task/workspace policy, return actionable structured errors, and test over the actual MCP transport.

Do not trust caller-supplied task identity when the server can inject it. Consider destructive-action confirmation, relationship reachability, pagination, and concurrent task state. Update [Automation and MCP](automation-and-mcp.md) when user capability changes.

## Build a plugin

Plugins are a peer extension mechanism to the seams above, aimed at
extensions that should ship and version independently of a kandev release.
A plugin backend is a Go binary that kandev spawns and supervises as a
subprocess, communicating over a strict typed gRPC protocol
(`internal/plugins/`, `pkg/pluginsdk`) — it receives bus events, serves
agent-invocable tools, and relays external webhooks, calling back into
kandev through a capability-gated Host RPC service (state, secrets,
cross-plugin events). A plugin may additionally ship an optional **native
frontend bundle** that the SPA loads at boot to register real routes, nav
items, slot components, and WebSocket handlers, sharing kandev's own React
instance and app store.

Plugins are distributed as a signed-or-unsigned tarball and installed by URL,
manual upload, or filesystem sideload/sync — there is no manifest-paste
registration step and no credentials to issue. The whole system sits behind
the `plugins` feature flag (Settings > System > Feature Toggles), off by
default in production. See [Plugins](plugins.md) for the operator-facing
install/operate flow, [Authoring a plugin](plugins-authoring.md) for the
build tutorial and SDK reference, and the [Plugin manifest
reference](plugins-manifest.md) for the complete `manifest.yaml` schema.

## Add a settings or workbench surface

Backend configuration should have a typed API and durable ownership before a web form is added. Web settings route through `apps/web/src/settings-routes.tsx`; task surfaces live in the relevant workbench components and state slices.

Cover initial load, no dependency, invalid credential, save, test, error, reconnect, and mobile behavior. Use status text in addition to color and make every control keyboard-accessible.

## Extension completion checklist

- Registry/startup wiring cannot silently omit the extension.
- Config and secrets have clear scope and redaction.
- Capability detection is truthful when optional dependencies are absent.
- Failures persist enough state for the user to recover.
- Cleanup is idempotent and does not delete unrelated state.
- Unit, integration, and user-visible E2E coverage exists.
- Public docs include setup, trust boundary, troubleshooting, and status.
- Release packaging contains every required binary, asset, or platform variant.

Related: [Architecture](architecture.md), [Backend development](backend-development.md), [Web development](web-development.md), and [Testing](testing.md).
