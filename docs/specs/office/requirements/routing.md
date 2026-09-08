---
status: deprecated
system: office
created: 2026-05-10
owners:
  - cfl
---
# Office Provider Routing Requirements

## Overview

Office agents need a predictable way to choose between CLI providers, accounts, and model strengths without cloning their role configuration. Users also need controlled fallback when a provider hits subscription or rate limits while preserving the Office agent's instructions, skills, permissions, budget, task, and worktree.

## Requirements

### REQ-OFFICE-ROUTING-001: Office Provider Routing

**Intent:** Office agents need a predictable way to choose between CLI providers, accounts, and model strengths without cloning their role configuration. Users also need controlled fallback when a provider hits subscription or rate limits while preserving the Office agent's instructions, skills, permissions, budget, task, and worktree.

#### Acceptance criteria

- **AC-OFFICE-ROUTING-001.1:** Provider routing is an advanced workspace setting and automatic fallback is disabled by default.
- **AC-OFFICE-ROUTING-001.2:** Office always resolves an execution profile from the agent's effective tier, even when automatic fallback is disabled.
- **AC-OFFICE-ROUTING-001.3:** When routing is disabled, Office selects only the first configured provider in the effective provider order for that tier. It does not health-filter, try a later provider, or silently use a workspace default profile.
- **AC-OFFICE-ROUTING-001.4:** Short same-route transient retry remains active when routing is disabled; only cross-provider fallback is disabled.
- **AC-OFFICE-ROUTING-001.5:** Every Office agent has an effective model tier even when routing is disabled.
- **AC-OFFICE-ROUTING-001.6:** Workspace settings provide the default model tier, initially `balanced`.
- **AC-OFFICE-ROUTING-001.7:** Agents inherit the workspace default tier unless the user sets an agent-specific override.
- **AC-OFFICE-ROUTING-001.8:** Workspace settings can define a global provider order, for example `claude -> codex -> opencode`.

## System design

The migrated technical source is split into [part 1](../system-design/routing-01.md), [part 2](../system-design/routing-02.md).
