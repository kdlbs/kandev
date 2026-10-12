---
status: active
system: disambiguate-waiting
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Disambiguate waiting

## Purpose

Tell an operator looking at the board whether a session that reads
`WAITING_FOR_INPUT` is actually waiting for them, or is still running work the
agent launched and left in the background. The visible outcome is the parked
affordance on the task card.

The capability is built from three parts: an **attestation** that the agent
launched detached work, a **liveness probe** that samples the host for that work,
and a **parked projection** that combines them with session state.

## Documents

| Document | Owns |
|---|---|
| [spec.md](spec.md) | Legacy source. The attestation, the probe's descendant walk, the parked projection formula, the revision epoch, and AC-40a through AC-81. Still authoritative for everything not listed below. |
| [requirements/orphaned-background-workloads.md](requirements/orphaned-background-workloads.md) | REQ-DW-ORPHAN-001, REQ-DW-ORPHAN-002 |
| [system-design/orphaned-background-workloads.md](system-design/orphaned-background-workloads.md) | Prior art, input inventory, platform capability matrix, E2E decision |

`migration: in_progress` is accurate and deliberate: `spec.md` remains the
editable source for the rest of this system. Only the orphan-attribution
capability has been extracted into the current layout.

## Terminology

- **Attested launch** — a tool call a registered recogniser classified as a
  detached background launch. Today the only recogniser is Claude's
  `shell_exec` with `Background=true`.
- **Turn start** — the wall-clock instant agentctl recorded for the current
  turn, truncated down to the platform's process start-time resolution before
  any comparison (legacy AC-80).
- **Descendant** — a process whose parent chain reaches the agent process.
  A workload stops being a descendant the moment its parent exits.
- **Orphan** — a process that was launched inside the session but whose parent
  has exited, so the kernel reparented it to init. It is no longer a descendant
  and the legacy walk cannot see it.
- **Session identity** — the value of the `KANDEV_SESSION_ID` environment
  variable, injected at agent launch and inherited by every descendant.

## Related systems

- [parked-notification-deferral](../parked-notification-deferral/spec.md) owns
  notification timing. Legacy AC-76 keeps notification independent of the parked
  projection, so nothing in this system can suppress or delay a notification.
