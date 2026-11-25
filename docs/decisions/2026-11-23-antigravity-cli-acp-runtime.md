# ADR-2026-11-23-antigravity-cli-acp-runtime (Prefer the installed Antigravity CLI)

**Status:** proposed

**Date:** 2026-11-23

**Area:** backend, protocol

## Context

Kandev's managed standalone server and the Antigravity CLI use separate Google sign-ins and conversation stores. Google added ACP to the CLI in version 1.5.0. Version 1.5.15 opened an authenticated ACP session and completed a prompt on the reported Kandev host. [Google ACP documentation](https://antigravity.google/docs/cli/acp)

## Decision

Keep one built-in Antigravity profile identity. Select the qualified CLI for new host-local sessions and model discovery. Treat discovery, command, and authentication as one runtime choice. Kandev supplies the vendor worker option for editor-supplied HTTP MCP. [Google ACP flags](https://antigravity.google/docs/ide/extensions/acp#flags)

If no CLI qualifies, use the complete standalone bundle for new sessions. Bind a saved conversation to the runtime that created it. Treat older Antigravity conversation records as standalone.

Google's executable owns its sign-in and private state. Kandev's install job owns the pinned standalone bundle. The device owner manages the external CLI through Google's installer and updater. [Runtime update authority](2026-10-01-agent-runtime-update-authority.md) · [Durable sessions](2026-09-10-durable-harness-session-boundaries.md)

## Consequences

Users with a signed-in CLI can use the same login in the built-in profile. Each runtime reports its own authentication and startup failures. Resume requires an available version of the original runtime and a Kandev build that understands its runtime association.

## Alternatives considered

- A second built-in agent would require users to duplicate or move profiles and workflow selections.
- Copying the CLI's credentials to the standalone server would depend on private credential formats and leave separate session stores.
- Bundling the CLI behind Kandev's install action requires a qualified artifact and a separate installation ownership contract. This change uses the installed CLI.
