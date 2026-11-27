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

## Keep Reviewers Off The Author's Provider

A review is an independent check only when the reviewer runs on a different provider from the one that did the work. Fallback can otherwise land the reviewer on that same provider. **Review on a different provider** prevents this for reviewer and approver runs that use legacy workspace provider routing with automatic fallback.

Turn it on under **Office > Routing > Review on a different provider**. The section lists every workflow step that seats a reviewer or approver, including the built-in Office workflow. Flip a step's switch, then select **Save changes** in the save bar at the bottom of the page. **Reset** discards unsaved changes. The setting is off by default.

In workflow YAML, the same setting is `exclude_author_provider` on the step's seat action, so export, import, and sync carry it:

```yaml
- type: ensure_participant_seat
  config:
    role: reviewer
    exclude_author_provider: true
```

For eligible runs, workspace routing skips the provider recorded for the task's latest Work-stage launch. This applies to the first attempt, fallback, and retries. A provider is the agent CLI, such as `claude-acp` or `codex-acp`, so two models run through the same CLI count as the same author.

- **No other provider available:** the run waits instead of reviewing its own work. Its route attempts show **skipped: wrote this task** for the author's provider. If another provider is only temporarily degraded, the run resumes when that provider recovers. If the author's provider is the only one in the order, the run waits for you to act.
- **No recorded author:** if no Work-stage run launched for the task, for example because a person did the work, the run launches normally.
- **Automatic provider fallback off:** the setting is not enforced.

Workspace routing records a `routing.author_exclusion` event for seat runs with the switch on. The event shows whether the rule excluded a provider or why the rule had no effect.

Kandev warns, without blocking, when the current setup cannot keep the promise. The warning appears at the top of **Office > Routing**, in the workflow import review, and in workflow sync results. Each warning says what will happen, lists the affected workflow steps, and says how to fix it. Steps with the same problem share one warning:

- **Automatic provider fallback is off:** reviews may run on the provider that did the work. Turn on **Automatic provider fallback**, or turn off **Review on a different provider** for the step.
- **Only one provider is configured:** reviews can never run on a different provider, so they wait until you act. Add a second provider, or turn the setting off.
- **A reviewer agent can only use one provider:** its reviews wait whenever the work ran on that provider. Give the agent another provider in its routing settings, or turn the setting off.
- **An agent has a bound execution profile.** Its runs follow that profile's routing policy and can use the author's provider. Configure the agent to use workspace provider routing to apply the seat's exclusion rule.

## What A Provider Switch Preserves

Kandev preserves the Office agent ID, task, run, task environment, worktree, instructions, and skills. The new execution profile supplies the provider CLI, account, model, mode, flags, environment, permission behavior, passthrough setting, and MCP configuration.

A provider-native session token is never reused by a different execution profile. Cross-provider fallback starts a fresh provider session and tells the new agent to inspect the durable task conversation, run state, and repository state before continuing. Provider chat history itself is not transferred.

For example, a custom CTO can normally run through a Codex GPT profile. When that route reaches its usage limit, an enabled fallback chain can launch a Claude Code Opus profile while the CTO's role, five assigned skills, instructions, task, and current worktree remain unchanged.

For a lightweight routine, provider fallback starts a fresh run-owned session for the next attempt. Delayed lifecycle events and usage from the previous provider are checked against their exact run session and cannot finish or charge the replacement attempt.
