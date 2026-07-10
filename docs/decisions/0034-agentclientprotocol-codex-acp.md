# 0034: Agent Client Protocol Codex ACP Bridge

**Status:** accepted
**Date:** 2026-07-10
**Area:** backend, protocol

## Context

Kandev's `codex-acp` agent previously launched `@zed-industries/codex-acp`. That bridge exposed an older Codex model catalogue and did not advertise newer Codex models already available in the native OpenAI Codex CLI. The actively maintained ACP bridge is published as `@agentclientprotocol/codex-acp`.

## Decision

Kandev's `codex-acp` agent launches `npx -y @agentclientprotocol/codex-acp` for ACP chat and one-shot inference sessions. The install script still installs `@openai/codex` for `codex login`, then installs the ACP bridge package for Kandev sessions.

No product spec update is required: the user-facing agent remains `codex-acp`; only the package that supplies the ACP bridge changes.

## Consequences

Codex ACP sessions receive the model catalogue and config options from the Agent Client Protocol bridge, including model IDs that the old bridge did not expose. Existing profiles using old model or mode IDs are reconciled by the profile healer: unavailable values are cleared or replaced with the probed current values. Auth method identifiers can differ between bridge implementations, so auth UI should keep consuming advertised methods instead of hard-coding bridge-specific IDs.

## Alternatives Considered

- Keep relying on `@zed-industries/codex-acp` and assume npm or local symlinks redirect it. Rejected because Kandev should declare the package it intends to execute and not depend on external aliasing.
- Use native `@openai/codex` passthrough only. Rejected because passthrough does not provide the ACP chat integration Kandev needs.
- Wait for the Zed package to publish a newer release. Rejected because the maintained successor package already exposes the needed Codex capabilities.
