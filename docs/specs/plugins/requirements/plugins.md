---
status: draft
system: plugins
created: 2026-04-26
owners:
  - cfl
---
# Plugin System Requirements

## Overview

Kandev keeps growing external integrations and surface-specific behavior directly in the core codebase: source-control sync, issue-tracker browsing, notification providers, and planned channel types (Slack, Discord, Telegram, email). Each one adds platform-specific logic — API clients, webhook handlers, payload formatting, OAuth flows, secret management, and bespoke UI — to the Go backend and the SPA. This creates three problems:

## Requirements

### REQ-PLUGINS-PLUGINS-001: Plugin System

**Intent:** Kandev keeps growing external integrations and surface-specific behavior directly in the core codebase: source-control sync, issue-tracker browsing, notification providers, and planned channel types (Slack, Discord, Telegram, email). Each one adds platform-specific logic — API clients, webhook handlers, payload formatting, OAuth flows, secret management, and bespoke UI — to the Go backend and the SPA. This creates three problems:

#### Acceptance criteria

- **AC-PLUGINS-PLUGINS-001.1:** Plugin **backends** are **Go binaries** distributed inside a release tarball (per-platform executables) that kandev **spawns and supervises as subprocesses** via `hashicorp/go-plugin`, speaking a strict typed **gRPC protocol** (`kandev.plugin.v1`) over a unix domain socket (macOS/Linux) or loopback TCP with AutoMTLS (Windows). No in-process backend loading, no separately-managed operator process, no HTTP transport for the backend contract.
- **AC-PLUGINS-PLUGINS-001.2:** A plugin MAY additionally ship a **native frontend bundle** (`ui.bundle`) that kandev loads into the SPA to register native routes/nav/components (see "Frontend plugin runtime"). This is the one in-process surface; the backend stays out-of-process (but is now kandev-managed, not operator-managed).
- **AC-PLUGINS-PLUGINS-001.3:** A plugin manifest declares identity, runtime executables (per OS/arch), capabilities, declared webhooks and authenticated actions, repository-provider and reference-source ownership, config schema, and optional UI bundle.
- **AC-PLUGINS-PLUGINS-001.4:** Plugins SHALL receive events, expose proxied external webhook endpoints, and read/write a plugin-scoped KV state — all over gRPC.
- **AC-PLUGINS-PLUGINS-001.5:** Plugins are distributed as a signed-or-unsigned release **tarball** and installed either by **URL** (kandev downloads it) or by **manual upload** (multipart file). There is no manifest-paste registration step.
- **AC-PLUGINS-PLUGINS-001.6:** The Settings > Plugins install dialog SHALL show the primary Install action as busy while an install is in flight, including an animated loading indicator and an installing label, while keeping the action disabled until the pipeline settles.
- **AC-PLUGINS-PLUGINS-001.7:** Capability-based access control: a plugin can only call Host RPCs it declared in its manifest; undeclared capabilities are rejected with a gRPC `PermissionDenied` status.
- **AC-PLUGINS-PLUGINS-001.8:** **Kandev owns the plugin process lifecycle**: it extracts the package, spawns the binary, performs the go-plugin handshake, health-checks it (`Ping`), and restarts it on crash or health-check failure. Operators no longer run or manage plugin processes themselves. The remote/self-hosted tier (`base_url` registration of an operator-run process kandev never spawns) is removed; see "Out of scope".
- **AC-PLUGINS-PLUGINS-001.9:** For a capability that needs workspace approval, Kandev shall authorize the request only when the manifest declaration, the current installation/workspace approval, and immutable Human-reserved policy all allow it. The Host shall enforce this intersection. A missing or revoked approval shall fail closed.
- **AC-PLUGINS-PLUGINS-001.10:** Each Host writer shall define and atomically revalidate the target versions and domain preconditions that apply to its command. A changed precondition shall return a typed conflict without a partial side effect.
- **AC-PLUGINS-PLUGINS-001.11:** A plugin shall access Kandev domain data and commands only through documented capability-gated Host contracts. Global MCP, private REST, direct database access, shell access, and prompt or comment text shall not confer plugin authority.
- **AC-PLUGINS-PLUGINS-001.12:** When a required Host capability or result state is unknown or unsupported, the plugin shall receive a typed unsupported result and shall not fall back to a broader integration path.
- **AC-PLUGINS-PLUGINS-001.13:** Kandev shall provide a connection-bound way for a plugin to discover supported capability families and approved workspace scope. The context shall describe authority but shall not grant authority. A missing, stale, or revoked context shall not grant authority. The Host shall enforce current approval on every request.

## System design

The migrated technical source is split into [part 1](../system-design/plugins-01.md), [part 2](../system-design/plugins-02.md), [part 3](../system-design/plugins-03.md), [part 4](../system-design/plugins-04.md).
