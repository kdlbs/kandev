---
title: "Extending Kandev"
description: "Add agents, executors, integrations, workflow behavior, MCP tools, plugins, settings, or workbench surfaces at their real ownership boundaries."
---

# Extending Kandev

An extension is complete when discovery/configuration, durable state, runtime behavior, recovery, security, tests, packaging, and public documentation agree. Registration and UI presence alone are not completion.

## Choose an extension seam

- **Agent:** add a CLI runtime or structured ACP integration.
- **Executor:** add a task environment and lifecycle backend.
- **Provider:** add an external service connection, health, and UI.
- **Workflow/MCP:** add durable automation behavior or a scoped tool.
- **Plugin:** ship an independently versioned extension.
- **Settings/workbench:** add a user-facing surface after backend ownership exists.

Use the smallest seam that owns the behavior, then follow the completion checklist at the end.

## Add an agent

For a local passthrough-only CLI, users can choose **Settings → Agents → Add TUI Agent**. That path persists a custom definition and default profile; no source patch is required.

To ship a built-in integration, add an `agents.Agent` implementation under `apps/backend/internal/agent/agents/` and register it in `internal/agent/registry/registry.go`. Structured agents currently need an ACP-speaking process. Optional interfaces add inference, CLI passthrough, native-binary preference, or interactive login.

Follow [Add an Agent CLI](add-agent-cli.md). Cover stable identity, installation detection, command construction, permissions, authentication, model/mode probing, resume, MCP delivery, remote runtime behavior, assets, registry tests, and packaging when a binary is bundled.

## Add an executor

Executors span product configuration and runtime implementation:

- `internal/task/models/models.go` defines persisted `ExecutorType`, `Executor`, and `ExecutorProfile`;
- `internal/agent/executor/` keeps product-to-runtime mappings typed;
- `internal/agent/runtime/lifecycle/executor_backend.go` defines the runtime backend;
- `internal/agent/runtime/lifecycle/env_preparer.go` separates environment preparation;
- `internal/backendapp/agents.go` registers backends and preparers;
- task repository defaults/handlers and web settings expose built-in profiles.

Local and worktree use the same standalone process backend but different materialization. Docker, remote Docker, SSH, and Sprites own distinct create/connect/cleanup behavior.

A production executor needs profile persistence and validation, scoped credentials, create/prepare/start/observe/stop/cleanup, agentctl delivery and resolution, multi-repository materialization, durable runtime IDs/status/errors, startup recovery, settings UI, and lifecycle/failure tests. Cleanup must be idempotent and must not delete another task's host, container, directory, or credential.

Do not advertise an executor until restart, cancellation, partial-create cleanup, connectivity, supported capabilities, cost, and credential boundaries are documented.

## Add a provider integration

Choose the closest existing provider domain under `apps/backend/internal/`. There is no universal configuration shape:

- Jira and Linear use workspace-scoped connection state;
- Sentry supports multiple named workspace instances;
- GitHub keeps installation-wide authentication with workspace settings, while GitLab owns one connection and its watches per workspace;
- Slack follows a workspace-oriented pattern.

A domain may contain a client, store/repository, service, provider, handlers, and poller. Shared helpers under `internal/integrations/` cover secret adapters, health polling, and workspace scope. Construction and non-fatal provider startup live in `internal/backendapp/`. Web clients/hooks live in `lib/api/domains/` and `hooks/domains/`; settings and shared provider UI live under `app/settings/` and `components/integrations/`.

Preserve the chosen global/workspace/instance scope through storage, secrets, routes, and UI. Validate custom hosts against SSRF, redact tokens and provider errors, handle pagination/rate limits, deduplicate polls/webhooks, expose health, and treat external titles/descriptions/comments as untrusted content. Add provider fakes and user-journey coverage.

## Add workflow or automation behavior

`internal/workflow/` owns workflow models, repository, service, engine, adapters, and transitions. `internal/automation/` owns workspace rules that can create or react to work. Preserve the distinction between a task transition and an automation trigger.

New actions or events need durable semantics, import/export compatibility when applicable, retry and duplicate handling, cycle prevention, UI editing, and tests using old saved workflow definitions.

## Add an MCP tool

`internal/mcp/server/server.go` owns tool schemas and availability by task, config, external, or office mode. Backend behavior lives in `internal/mcp/handlers/`; registration and dependencies are wired from `internal/backendapp/`. Agentctl hosts MCP transports and relays calls over the agent stream.

A new relayed tool normally requires:

1. a schema, tool name, and mode assignment in the server;
2. a WebSocket action in `apps/backend/pkg/websocket/actions.go`;
3. a handler and registration in `internal/mcp/handlers/`;
4. server mode/count, handler, transport, and integration tests;
5. an update to [Automation and MCP](automation-and-mcp.md) when capability changes.

Inject task/session identity from server context instead of trusting arguments. Enforce task/workspace reachability, confirmation for destructive actions, pagination, concurrency behavior, and least-privilege credentials. The backend's external MCP routes currently have no Kandev user-auth middleware; deployment network controls are part of the security boundary.

## Build a plugin

Plugins are a peer extension mechanism to the seams above, aimed at
extensions that should ship and version independently of a kandev release.
A plugin backend is a Go binary that kandev spawns and supervises as a
subprocess, communicating over a strict typed gRPC protocol
(`internal/plugins/`, `pkg/pluginsdk`) — it receives bus events and relays
external webhooks, calling back into kandev through a capability-gated Host
RPC service (state, secrets, read-only data, cross-plugin events). A plugin may additionally ship an optional **native
frontend bundle** that the SPA loads at boot to register real routes, nav
items, slot components, and WebSocket handlers, sharing kandev's own React
instance and app store.

Plugins are distributed as a signed-or-unsigned tarball and installed by URL,
manual upload, or filesystem sideload/sync — there is no manifest-paste
registration step and no credentials to issue. Plugins are part of the base
product and are not a runtime feature toggle. See [Plugins](plugins.md) for the operator-facing
install/operate flow, [Authoring a plugin](plugins-authoring.md) for the
build tutorial and SDK reference, and the [Plugin manifest
reference](plugins-manifest.md) for the complete `manifest.yaml` schema.

## Add settings, flags, or workbench UI

Create backend ownership and validation before a web form. Web settings require the appropriate domain client/hook, `src/settings-routes.tsx`, page/components, and sidebar or general navigation. Add a global state slice only when multiple surfaces or WebSocket hydration require it.

Wire a release toggle through every enforcement layer in one change:

1. Add a `KANDEV_FEATURES_*` entry to `profiles.yaml` with `prod`, `dev`, and
   `e2e` set to `false` until the rollout is ready.
2. Add the typed `FeaturesConfig` field with explicit `mapstructure` and `json`
   tags, then add one metadata/read/apply registration in
   `internal/runtimeflags/registry.go`. The registry owns the binding; do not
   add a parallel per-flag map or switch.
3. Gate backend construction, handlers, HTTP/WS/MCP actions, background jobs,
   and agent prompt paths at their narrow composition boundary. The disabled
   path must fail closed before deriving data, writing state, or dispatching
   work, while existing readable data remains inert.
4. Add the field to the frontend `defaultFeatureFlags` declaration, use
   `useFeature` for client surfaces, and call `notFound()` from the server
   layout or page when a guessed URL must remain unavailable.
5. Add backend and frontend tests for enabled and disabled behavior, including
   legacy behavior while the flag is off. Shared refactors, migrations, and
   data reads are not automatically protected by a feature flag.

For rollout, merge with the flag off, enable it on one install through an
admin override or explicit environment and restart, then promote the `prod`
default after observation. Remove the live flag only after it is permanent,
and move its key and environment variable to the append-only retired identities
in `internal/runtimeflags/registry.go`. Never reuse either identity for
different behavior. A flag is incomplete if any layer can expose or execute
the feature while it is disabled. Do not turn an internal package or
environment variable into a public setting without first defining its
compatibility contract and support status.

Workbench changes must preserve task/session/repository selection, dock-layout restoration, reconnect behavior, and the separate mobile layout. Test unavailable dependencies, invalid credentials, save/test failures, keyboard access, and narrow viewports.

## Completion checklist

- Startup/registry wiring cannot silently omit the extension.
- Persisted types, defaults, migrations, APIs, and UI use one stable identity.
- Config and secrets have explicit global/workspace/task/instance scope and redaction.
- Optional dependencies produce truthful capability and health state.
- Retry, recovery, cancellation, and idempotent cleanup are defined.
- Cross-process and wire compatibility is tested.
- Unit, integration, and required Playwright coverage pass.
- Release bundles contain every required binary, asset, image, and platform mapping.
- Public docs cover setup, status, trust boundary, limits, and troubleshooting.

Related: [Architecture](architecture.md), [Backend development](backend-development.md), [Web development](web-development.md), and [Testing](testing.md).
