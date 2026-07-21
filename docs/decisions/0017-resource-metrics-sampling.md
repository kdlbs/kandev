# 0017: Resource Metrics Sampling

**Status:** accepted
**Date:** 2026-06-14
**Area:** backend, frontend, protocol

## Context

Kandev needs lightweight CPU, memory, disk, load, temperature, and IO pressure visibility for users running the app on a remote server or using isolated execution environments. The values should appear in the opt-in global status surface. Kandev also runs agentctl in multiple runtime shapes: some share the backend host, while Docker, remote, and cloud executors have their own resource boundary.

## Decision

Use global system metrics settings for collection policy and per-user settings only for display preference. Global Kandev settings are persisted in the shared `settings` key/value table owned by `internal/system/settings`; resource metrics store their sampler policy under the `system_metrics` key instead of owning a feature-specific settings table. The backend sampler starts only while at least one desktop/tablet WebSocket connection has explicitly subscribed to metrics display, and stops when the last interested connection unsubscribes or disconnects.

The backend process samples the backend host. Agentctl exposes an execution-environment metrics endpoint for runtimes with a distinct boundary: local Docker, remote Docker, SSH/remote VPS, and cloud/Sprites. Local process and worktree agentctl instances are not sampled separately in v1 because they duplicate the backend host. Containerized agentctl collectors prefer cgroup CPU and memory accounting when available, falling back to procfs only when no meaningful cgroup limit exists.

Metrics updates are delivered over a dedicated WebSocket stream to subscribed connections only, not via global broadcast. Desktop and tablet render only the Kandev backend-host source in the app status surface; phone subscribes and renders that same host source only while the global Status drawer is open. Execution-source sampling remains a backend capability, but the global status surface does not couple its built-in presentation to an active task or session. Plugins may use host state for separately owned context-specific presentations.

## Consequences

The backend avoids background procfs/cgroup/agentctl polling when no visible UI needs metrics. Multi-user behavior stays clear: global settings control what is sampled and how often, while each user controls whether they see it. Docker and remote execution values remain available in the metrics stream without making the global status surface task-dependent.

This adds a small connection-interest registry, a reusable install-wide settings table, and a new agentctl API surface. The status surface makes host metrics available consistently across hosted routes and deliberately ignores active-session execution sources. A closed phone Status drawer creates no sampling interest.

## Alternatives Considered

- **Per-user sampler settings:** Rejected because metric collection is process-wide and would cause conflicting intervals and duplicated polling in multi-user sessions.
- **Always-on backend sampler:** Rejected because the feature is optional and should have zero steady-state cost when nobody displays it.
- **Sample every agentctl instance:** Rejected because local process/worktree instances share the backend host and would duplicate values. V1 samples only execution boundaries that add distinct resource context.
- **Global broadcast:** Rejected because this is a high-frequency stream; only interested connections should receive it.
- **Metrics-owned `system_settings` table:** Rejected because future install-wide Kandev settings should share one clearly owned backend store instead of each feature inventing its own generic table.
