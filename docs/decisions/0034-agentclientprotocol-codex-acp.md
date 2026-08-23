# 0034: Agent Client Protocol Codex ACP Bridge

**Status:** accepted (amended 2026-07-26)
**Date:** 2026-07-10
**Area:** backend, protocol

## Context

Kandev's `codex-acp` agent previously launched `@zed-industries/codex-acp`. That bridge exposed an older Codex model catalogue and did not advertise newer Codex models already available in the native OpenAI Codex CLI. The actively maintained ACP bridge is published as `@agentclientprotocol/codex-acp`.

## Decision

Kandev's `codex-acp` agent launches the exact effective version of
`@agentclientprotocol/codex-acp` for ACP chat and one-shot inference sessions.
The reviewed Kandev default is currently `1.6.0`; an install-wide operator
selection can override it. Normal launches prefer npm's execution cache. The
install script still installs `@openai/codex` for `codex login`; that native
authentication helper is separate from the managed ACP runtime.

The host operator can deliberately refresh the ACP package through
**Settings > Agents > Update agent**. Kandev then re-probes the bridge and
replaces the advertised model and configuration catalogue for future
sessions. The shared runtime-management boundary is recorded in
[ADR-2026-07-26](2026-07-26-user-managed-agent-runtime-updates.md) and the
[managed runtime update spec](../specs/agents/runtime-updates.md).

## Consequences

Codex ACP sessions receive the model catalogue and config options from the Agent Client Protocol bridge, including model IDs that the old bridge did not expose. Existing profiles using old model or mode IDs are reconciled by the profile healer: unavailable values are cleared or replaced with the probed current values. Auth method identifiers can differ between bridge implementations, so auth UI should keep consuming advertised methods instead of hard-coding bridge-specific IDs.

Codex-specific ACP `cli_flags` are not advertised because the bridge entrypoint does not apply the native Codex `-c` config overrides to chat sessions. Kandev still exposes the universal agentctl auto-approve toggle for ACP permission requests and keeps native Codex flags scoped to passthrough mode.

Kandev source supplies a reviewed default and records any operator-selected
exact version. Incident diagnosis also uses the version reported during ACP
initialization. Compatibility is enforced through ACP protocol negotiation and
advertised capabilities, so an upstream same-protocol regression can still
require operator intervention.

## Alternatives Considered

- Keep relying on `@zed-industries/codex-acp` and assume npm or local symlinks redirect it. Rejected because Kandev should declare the package it intends to execute and not depend on external aliasing.
- Use native `@openai/codex` passthrough only. Rejected because passthrough does not provide the ACP chat integration Kandev needs.
- Wait for the Zed package to publish a newer release. Rejected because the maintained successor package already exposes the needed Codex capabilities.
