---
status: active
system: agents
created: 2026-10-30
owners:
  - flameOnYou
---

# Codex Subagent Wire Compatibility Requirements

## Overview

Codex runtime upgrades must preserve the existing subagent presentation contract.
The agent system owns provider wire interpretation; task storage and chat consume
the resulting provider-neutral payload. This restores recognition lost when the
managed Codex ACP runtime changed from 1.13.1 to 2.2.2 (issue #4953).

## Requirements

### REQ-AGENTS-CODEX-SUBAGENT-WIRE-001: Stable subagent recognition

**Intent:** Users can identify delegated children and their reported state across
supported Codex ACP wire formats.

#### Acceptance criteria

- **AC-AGENTS-CODEX-SUBAGENT-WIRE-001.1:** Legacy metadata and complete structured
  Codex spawn/start input shall produce the existing subagent payload, preserving
  reported child identity, name, prompt, model, and state when available.
- **AC-AGENTS-CODEX-SUBAGENT-WIRE-001.2:** Observed updates and terminal activities
  shall retain existing card correlation, sibling isolation, immutable snapshots,
  and parent identity. An unmatched terminal activity shall not create a card.
- **AC-AGENTS-CODEX-SUBAGENT-WIRE-001.3:** Incomplete structured input, title-only
  signals, control operations, and other providers shall not gain Codex subagent
  recognition. Explicit legacy metadata shall remain authoritative.
- **AC-AGENTS-CODEX-SUBAGENT-WIRE-001.4:** Structured subagent evidence shall retain
  the existing capacity-continuation fence without attesting background liveness.

## Out of scope

Native ACP child-session subscriptions, full child transcripts, new tabs,
historical data backfills, runtime version changes, and UI redesign.
