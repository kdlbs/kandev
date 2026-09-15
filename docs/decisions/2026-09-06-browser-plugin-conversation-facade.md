# Browser plugins read session conversations through a typed Host facade

**Status:** proposed
**Date:** 2026-09-06
**Area:** frontend, backend, protocol, plugins, security

## Context

ADR 0047 gives a plugin backend sanitized conversation reads through the capability-gated Host gRPC channel. Native browser plugins have a different gap: prompt history currently reads private Zustand projections, first-party REST helpers, raw session WebSocket events, local mobile navigation state, custom-prompt settings, and a browser-only favorites store.

An external prompt-history plugin could technically reach some of these because native plugin JavaScript runs in Kandev's origin and the runtime still exposes a compatibility `host.store`. That would make the plugin depend on unversioned internal shapes, duplicate subscription race handling, and break mobile navigation. Calling a plugin backend for every page would avoid private frontend state but would require a Go backend for an otherwise browser-only feature and add an unnecessary browser-to-plugin-to-host round trip.

## Decision

Add a typed browser `host.conversation` facade and scoped task-panel navigation capability.

- Browser reads use authenticated, plugin-scoped HTTP routes under `/api/plugins/{pluginId}/conversation/...`. The routes require an active plugin with `api_read:messages`, enforce normal user/session authorization, and call existing task services.
- The facade exposes React hooks for paginated session messages, session turns, and read-only favorite state. It owns WebSocket readiness, live reconciliation, reconnect, request cancellation, and plugin-generation fencing.
- Public DTOs are narrower than first-party message and turn DTOs. Content is sanitized; raw system content and arbitrary metadata never leave the host. The only metadata-derived field is a validated sender task ID.
- Task-panel props gain a generation-bound `conversation.openMessage(messageId)` capability. It is scoped to the panel's current task/session/presentation and delegates to the native desktop or mobile navigation owner.
- Host-owned prompt-alias rendering is exposed as a curated UI component so custom-prompt contents and responsive preview mechanics stay private.
- The existing Go Host `Messages().List` RPC remains the plugin-backend contract. It is not stretched to serve browser state, turns, favorite state, or navigation.

## Consequences

- A UI-only plugin can reproduce prompt history without importing Kandev web modules, reading `AppState`, using raw WebSocket handlers, or shipping a backend binary.
- Kandev remains responsible for authorization, content sanitization, subscription gaps, native navigation, and private user-state semantics.
- The supported browser contract is capability-gated, but native same-origin plugins are still not a hard security sandbox. This decision does not claim otherwise.
- Additive SDK and task-panel fields remain compatible with existing plugins. Consumers require a `min_kandev_version` that includes the facade.
- Core prompt history remains until a separate plugin and extraction package prove parity and migrate saved built-in panel identities.

## Alternatives considered

- **Expose the current prompt-history Zustand slice.** Rejected: it freezes a private cache, actions, and generation model into the public SDK and gives plugins unrelated mutable application state.
- **Tell plugins to call `/api/v1/task-sessions/...` and register raw WS handlers.** Rejected: it makes internal REST/event payloads public by accident and leaves every plugin to reproduce snapshot/subscription race handling.
- **Require a plugin backend to proxy Host gRPC reads.** Rejected: it adds a process and network hop to a browser-only panel, still does not solve native navigation or browser-local favorites, and prevents a UI-only package.
- **Add prompt-history-specific REST and React components as one monolithic Host widget.** Rejected: it would move the feature into a differently named core component rather than establish reusable conversation and panel boundaries.
- **Move mobile scroll-target state into global Zustand.** Rejected: the current mobile layout owns its active full-height surface and exact scroll target locally. A scoped injected adapter preserves that ownership and prevents stale global navigation.
