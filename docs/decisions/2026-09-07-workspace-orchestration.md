# Workspace orchestration owns its coordination runtime

**Status:** accepted
**Date:** 2026-09-07
**Area:** backend, frontend

## Context

Office's model routing and worker-persona setup duplicate normal task execution-profile assignments. Users need conversational workspace coordinators without a separate workspace, board or task execution engine.

## Decision

Introduce workspace orchestrator registrations and reusable behavioral roles under a distinct experimental flag and API. Registrations use core agent profiles and own their instructions, memory and conversation mappings. An independent coordination runtime uses the core run dispatcher, task execution and scoped runtime authentication. Workspace settings own instances; global Orchestration settings own global roles. Role names, icons and instructions are global; assignments add execution identity and workspace context. Saved role updates apply on the next turn.

Registered coordinators delegate to normal execution profiles through canonical Kanban task operations. The registration is resolved server-side; agents cannot request direct-profile semantics through a body flag. An orchestrator's own profile remains pinned without pinning task profiles. Task ownership metadata routes lifecycle notifications to exactly the coordinating instance. Runtime and compatibility routes check the independent feature flag.

Conversation presentation uses the standard app shell and shared chat components without Office issue properties. Task navigation returns to its coordinator on desktop and mobile. The mobile link lives inside the fixed task top bar, rather than behind it.

## Consequences

Office remains compatible and separately gated. Orchestration exposes its own runtime API and MCP conversation surface; its backend dependency graph contains no Office packages. Core run models, storage, dispatch and runtime authentication are shared infrastructure. Office cannot address registered Orchestration personas or conversations. An explicit import endpoint preserves existing assistant identity and conversation history. New orchestration configuration never creates a delivery workflow. Existing workflow execution, account authentication, permissions and external repair access remain Kandev's existing systems.
