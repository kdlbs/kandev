# ADR-2026-09-27-backend-dialed-detached-agents: Backend-dialed links and waiting gates for detached agents

**Status:** accepted
**Date:** 2026-09-27
**Area:** protocol

## Context

On 2026-09-27 an SSH-executor agent lost its backend link at 02:00. A VPN
reset left the tunnel dead for about 2 minutes. The SSH keepalive watchdog
tore the transport down after 45 seconds, and nothing reconnected. The agent
worked on for about 8 hours. Every Kandev MCP call failed in 5 seconds, and
the agent looped on sleep and retry while the chat showed nothing. A later
resume sent ACP `initialize` to the busy agent and force-stopped it.

Durable agent delivery (PR #3598) adds an agentctl journal and cursor replay.
It does not re-establish a lost transport, and it does not change the
agent-to-backend MCP path. A decision is needed on four questions:

- which side opens the link;
- what a gate operation does without a backend;
- what bounds an unreachable agent;
- when a disconnected run counts as lost.

CI/CD runners were surveyed as prior art: GitHub Actions, Azure Pipelines,
GitLab Runner, Buildkite, Jenkins, Kubernetes, Nomad, and Temporal. They
agree that a job keeps running while the coordinator is unreachable, and that
gates are evaluated on the coordinator. They differ on who dials, and on
whether a returning runner is re-adopted or killed.

## Decision

1. **The backend opens every link.** agentctl never dials the backend. After
   a loss, the backend redials the executor transport, triggered by
   reachability recovery, capped exponential backoff, or a user action.
2. **Gates wait. They are never queued.** A Kandev MCP call made while
   detached waits for reattach and is processed by the backend then. agentctl
   never applies a gate, keeps no outbox, and never resends a call whose
   outcome is unknown.
3. **An offline budget bounds the agent.** After 15 minutes detached (an
   executor profile can override this), agentctl cancels the running turn and
   keeps its journal. The runner fences itself, because it is the only side
   that can act during the outage.
4. **Time never declares a run lost.** A disconnected run ends only when the
   link returns, a reconnect finds the agent gone, or the user stops it. A
   returning agent is re-adopted, never killed for being late.

## Consequences

- A short network drop becomes a visible, self-healing state rather than a
  failed turn. This builds on durable delivery's replay instead of adding a
  second transport.
- The laptop-hosted backend exposes no inbound listener and needs no new
  authentication.
- Each remote executor must implement a redial. Kubernetes already has one.
  SSH, remote Docker, and Sprites need new code.
- Budget expiry costs the running turn. The user resumes with an explicit
  instruction after reconnect.
- The agentctl unowned reaper must not stop agentctl before the budget
  expires.
- A backend restart while disconnected still falls back to the existing
  resume path.

## Alternatives Considered

- **agentctl dials the backend (every surveyed CI runner does this).** Those
  coordinators are stable, authenticated public services. Kandev's backend is
  often a laptop that sleeps, changes networks, and serves `/mcp` without
  authentication. Accepting inbound links would require authentication
  first. The side whose network changes should be the side that dials.
  Jenkins controller-to-agent links are the closest precedent.
- **Queue gate calls in an agentctl outbox and replay them on reconnect.** A
  queued step completion would advance the board after the fact, with nobody
  having seen the work. It also turns every non-idempotent call into a
  duplication risk.
- **Fail Kandev calls fast, as today.** The agent cannot tell a 2-minute drop
  from a permanent failure. The incident shows it invents polling loops.
- **Freeze the agent process (SIGSTOP) at budget expiry.** The model API
  connection usually times out while frozen, so the turn tends to fail on
  resume anyway.
- **No offline budget.** Gates alone would bound only agents that call Kandev.
  An agent doing long local work would run unbounded while nobody can see or
  stop it.
- **Declare the run lost after a window, then kill a late runner (GitHub
  Actions, GitLab, Buildkite).** An agent turn holds hours of context, so
  re-adoption is worth more than a clean kill. Nomad's `keep_original` and
  Jenkins durable steps take the same position.
