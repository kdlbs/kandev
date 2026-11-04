---
title: "Office Provider Routing"
description: "Route persistent Office agent identities through interchangeable provider execution profiles."
status: experimental
---

# Office Provider Routing

Office provider routing separates an agent's persistent role from the CLI profile used to run it. This lets a CTO, reviewer, or other custom Office agent keep the same instructions, skills, permissions, budget, task ownership, and history while Kandev selects a Codex, Claude Code, or another configured execution profile for each launch. Lightweight routine runs use the same routing choices without inventing a task: each fire has its own run-owned session and attempt identity.

Office mode is currently feature-flagged. The routing settings appear under **Office > Routing** when Office is enabled.

Start with [Get started with Office](office-getting-started.md) to enable Office and create a workspace. Use [Configure an Office agent team](office-agents.md) for roles, instructions, skills, and permissions, and [Office operations](office-operations.md) for blocked or failed runs.

## Configure Routes

1. Create the concrete agent profiles you want to launch. Each profile contains one provider CLI, account environment, model, mode, flags, permission behavior, passthrough setting, and MCP configuration.
2. Open **Office > Routing**.
3. Add providers in the order Kandev should consider them.
4. For each provider, select an existing runtime execution profile for the Frontier, Balanced, and Economy tiers you want that provider to serve. Rich Office agent identities are not valid routing targets.
5. Choose the workspace default tier and save.
6. Optionally override the tier or provider order on an individual Office agent.

The selectors store execution-profile IDs. Model names shown in the editor are derived from those profiles; entering a model name alone is not enough to define a launch.

## Fallback Behavior

**Automatic provider fallback off** still uses the first configured execution profile for the requested tier. Kandev does not health-filter the route or try later providers.

**Automatic provider fallback on** evaluates configured profiles in order. A provider that reports a recognized authentication, quota, rate-limit, outage, subscription, or model-availability failure can be degraded and skipped for later launches. Kandev then tries the next healthy configured profile. Agent-level provider-order overrides constrain this list; an agent pinned to Claude does not fall back to Codex.

If no route is available, Kandev keeps the task assigned to the same Office agent and waits for provider capacity or user action. Retryable limits are retried automatically after their retry time. Credential and configuration failures remain blocked until corrected or retried manually.

When the refusal comes from the install-wide automatic session ceiling rather than provider health, the pending launch is still owned by Kandev's ceiling retry sweep, not by the Office scheduler. The run stays visibly parked; once capacity frees, the sweep replays the stored launch and binds its session to the run. If that bind write fails, the launch's ready event retries the same bind. A terminal event can also recover the exact run if it arrives first. If the launch fails, Office applies its normal failure policy; cleanup cannot report the failed launch as successful. If the repository stays unavailable, the run remains parked and visible. Do not clear the run's routing block by hand to retry it: a manual clear re-queues the same run and can launch a second session beside the ceiling's replay. If the ceiling drops the stored launch because it can no longer be replayed, the run is failed with that reason. A failed drop-settlement write keeps the deferral for another ceiling sweep.

A launch rejected because its built agent or continue command is invalid fails the run immediately and reports the error, without consuming the retry budget. Correct the profile command before retrying. Temporary provider and workspace failures retain their existing recovery behavior.

## What A Provider Switch Preserves

Kandev preserves the Office agent ID, task, run, task environment, worktree, instructions, and skills. The new execution profile supplies the provider CLI, account, model, mode, flags, environment, permission behavior, passthrough setting, and MCP configuration.

A provider-native session token is never reused by a different execution profile. Cross-provider fallback starts a fresh provider session and tells the new agent to inspect the durable task conversation, run state, and repository state before continuing. Provider chat history itself is not transferred.

For example, a custom CTO can normally run through a Codex GPT profile. When that route reaches its usage limit, an enabled fallback chain can launch a Claude Code Opus profile while the CTO's role, five assigned skills, instructions, task, and current worktree remain unchanged.

For a lightweight routine, provider fallback starts a fresh run-owned session for the next attempt. Delayed lifecycle events and usage from the previous provider are checked against their exact run session and cannot finish or charge the replacement attempt.
