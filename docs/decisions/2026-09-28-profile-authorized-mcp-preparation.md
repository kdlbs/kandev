# ADR-2026-09-28-profile-authorized-mcp-preparation: Profile-authorized native MCP preparation

**Status:** accepted
**Date:** 2026-09-28
**Area:** backend

## Context

Copying agent-native MCP definitions and valid credentials does not make a fresh
workspace usable when the agent also requires workspace-specific server approval.
The user wants task startup governed by agent profile options, without manual
CLI setup. Other agents may have different discovery and approval mechanisms.

## Decision

The import-enabled profile grants standing authorization to prepare and approve
only the final eligible Kandev-owned imports. Agent-neutral selection, progress
and failure contracts use explicit native adapters; Cursor is the first adapter.
Native source/task disables and executor policy remain vetoes. Server connection
approval does not grant individual tool-execution permission.

Use the native server-specific approval command before conversational startup,
with final-definition checks, bounded output and no chat messages. Fresh OAuth
consent remains a user action exposed through Kandev's task terminal/browser flow.
Login commands use an explicitly persisted one-shot terminal launch policy.
Socket reconnection does not authorize another OAuth attempt; an explicit
Authenticate action creates a new attempt after the prior process ends.
In-flight requests reuse the same terminal. Commands remain server-side.

## Consequences

Fresh task workspaces can use existing credentials without user commands.
Preparation gains diagnosable approval/auth/connection states. Native CLI
compatibility needs versioned tests. Credential refresh continuity must preserve
native updates without retaining removed sources indefinitely.

## Alternatives Considered

- Blanket `--approve-mcps`: also trusts unrelated project definitions and hides
  the profile's selected-import boundary.
- Direct approval-file synthesis: couples Kandev to private hash/storage formats.
- Manual per-task commands: leaves the seamless-start contract unsatisfied.
- Generic shell hooks supplied in profiles: exposes unnecessary arbitrary-command
  configuration and prevents typed status/recovery behavior.

## Related design

- [Agent MCP preparation](../specs/agents/system-design/agent-mcp-preparation.md)
