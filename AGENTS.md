# Kandev Engineering Guide

> **Purpose**: This document captures up-to-date architecture notes, agent protocols, and frontend data-fetching patterns for Kandev.
>
> **Related**: [ARCHITECTURE.md](ARCHITECTURE.md), [docs/asyncapi.yaml](docs/asyncapi.yaml)

## Repo Layout

- `apps/backend/`: Go backend (WebSocket gateway, orchestrator, agent lifecycle, agentctl build).
- `apps/web/`: Next.js frontend (SSR + WS + Zustand store).
- `apps/packages/`: Shared packages/types.
- `apps/landing/`: Marketing/landing site that shares UI components with the web app.
- `apps/backend/dockerfiles/`: Agent container images (e.g., augment-agent).

## Tooling and Workspace

- **Package manager**: `pnpm` workspace.
- **Workspace root**: repository root.
- **Web app**: `apps/web` (Next.js, client + server components).
- **Backend**: `apps/backend` (Go services, agent lifecycle, ws).
- **UI**: Shadcn-based components are wrapped under `@kandev/ui`.

Common commands:

```bash
pnpm -C apps/web dev
pnpm -C apps/web lint
pnpm -C apps/web typecheck

make -C apps/backend test
make -C apps/backend lint
```

## Backend Architecture (High Level)

- **Transport**: WebSocket-only for client <-> backend.
- **Gateway**: Single WS endpoint at `ws://localhost:8080/ws`.
- **Orchestrator**: Starts/stops/resumes tasks, manages sessions.
- **Agent lifecycle**:
  - Supports Docker and standalone.
  - Creates agentctl instances for workspace access (shell, git, file ops).
  - Starts agent subprocesses explicitly.
- **agentctl**:
  - HTTP server for per-agent instance management.
  - Uses ACP (JSON-RPC 2.0 over stdin/stdout) to talk to agent process.
  - Streams ACP updates and workspace outputs to backend.

### Runtime / agentctl Flow (Simplified)

```
Client (WS)             Backend                Runtime              agentctl             Agent
   | orchestrator.start |                      |                    |                    |
   |------------------->| LaunchAgent          | Create instance    |                    |
   |                    |--------------------->|------------------->|                    |
   |                    | StartAgentProcess    | Configure/Start    |----> agent process |
   |                    | ACP calls            |                    |<--- ACP messages   |
   |<-------------------| WS notifications     |                    |                    |
```

### Protocols

- **ACP**: JSON-RPC 2.0 over stdin/stdout (agent <-> agentctl <-> backend).
- **WS Message Format**:

```json
{
  "id": "uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": { "...": "..." },
  "timestamp": "2026-01-10T12:00:00Z"
}
```

- See `docs/asyncapi.yaml` for WS actions and payloads.

## Frontend Architecture

### Key Principles

- **Store is the source of truth** (Zustand store slices).
- **SSR preloads data** into the store via `StateHydrator`.
- **Components read from store only**; avoid ad-hoc fetches in UI components.
- **Reusable components** preferred; split large pages into smaller components.
- **Data fetching should live where it is required**; avoid leaking fetches into parent controllers.

### Data Fetching and SSR

- SSR fetch in layout/page, map to store shape, hydrate via `StateHydrator`.
- WebSocket events update store slices for real-time sync.
- Only re-fetch when WS cannot deliver full payloads (document why).

### Subscription Strategy

TODO

**Why This Works**

- Components declare their dependencies explicitly.
- Subscription cleanup is automatic on unmount.
- WebSocket client guarantees only one real subscription per channel.
- Store updates flow globally, components re-render only on relevant slices.

### Three-Layer Model

```
Layer 1: WebSocket Client (connection + dedup)
Layer 2: Event Handlers (route WS payloads to store)
Layer 3: Components (subscribe + render)
```

### Store Structure

TODO

### Custom Hooks Pattern

TODO

### Page-Level Responsibilities

TODO

## Agents and ACP

### ACP Basics

- **Transport**: JSON-RPC 2.0 over stdin/stdout.
- **Format**: newline-delimited JSON.
- **Direction**: request/response + notifications.

**Backend → Agent (Requests)**
- `initialize`
- `session/new`
- `session/load`
- `session/prompt`
- `session/cancel`

**Agent → Backend (Notifications)**
- `session/update` (progress/content/tool calls/complete/error)

### agentctl HTTP (Internal)

- `/health` GET
- `/api/v1/status` GET
- `/api/v1/start` POST
- `/api/v1/stop` POST
- `/api/v1/acp/*` (initialize, session/new, session/load, prompt, stream)
- `/api/v1/shell/*` (status, stream, buffer)
- `/api/v1/workspace/*` (git status, file changes)

### Agent Types

- Augment agent lives at `apps/backend/dockerfiles/augment-agent/`.
- Registry defaults in `apps/backend/internal/agent/registry/defaults.go`.

## Frontend UI System (Shadcn)

- Prefer `@kandev/ui` components over ad-hoc HTML.
- Reuse components; avoid duplicating UI patterns.
- Split large pages into smaller components; keep layout vs data vs presentation separated.

## Best Practices

- Prefer store-driven state over local fetches.
- Prefer reusable components over duplication; split large pages into smaller child components.
- Keep data fetching scoped to the component that needs it; avoid leaking fetches into parent controllers.
- Avoid WS request/response inside UI components when store actions can handle it.
- Keep SSR fetches minimal and map responses into store-friendly shapes.
- Component subscriptions should be local; WS client should dedupe.
- Keep agents logging to stderr; stdout only for ACP.

## agentctl Server Architecture (Adapter Model)

The agentctl server now uses a protocol adapter layer to support multiple agent CLIs while keeping a consistent HTTP + WebSocket surface for the backend.

- `server/adapter/` defines the `AgentAdapter` interface and normalized `SessionUpdate` payloads.
- `ACPAdapter` uses the ACP JSON-RPC protocol over stdio (`server/adapter/acp_adapter.go`).
- `CodexAdapter` supports Codex-style JSON-RPC over stdio (`server/adapter/codex_adapter.go`).
- `process.Manager` owns the subprocess, wires stdin/stdout into the selected adapter, and forwards normalized updates to API streams.
- `api/acp.go` exposes protocol-agnostic endpoints (`/api/v1/acp/*`) that delegate to the adapter.
- `instance/` manages per-agent instance servers on dedicated ports (control server creates instances; each instance hosts its own API server).

---

**Last Updated**: 2026-01-11
