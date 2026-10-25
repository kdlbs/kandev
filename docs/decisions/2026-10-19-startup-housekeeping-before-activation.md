# ADR-2026-10-19-startup-housekeeping-before-activation: Visible preparation before runtime activation

**Status:** accepted
**Date:** 2026-10-19
**Area:** backend

## Context

A normal restart appeared usable before the viewed conversation began restoring.
The measured boot took 32.59 seconds to readiness, with 8.41 seconds spent in
prompt-sequence backfill and about 15 seconds without individual housekeeping
timing. Startup reconciled 100 retained runtime records although the user had
only two working tasks. Retained records are not the number of active agents.
Some producers activate before global preparation completes, and independent
background conversation restorations currently execute serially.

The user selected one combined change: perform required global housekeeping
before normal activation, show named steps with durations like backup/migrations,
and remove unnecessary recurring historical scans. The existing minimal server
must remain available to render status. A remote session's availability is a
separate matter from installation readiness.

## Decision

Make preparation and activation explicit lifecycle boundaries. Complete required
local persisted-state housekeeping before launch producers or restoration
activate, and report it through the existing startup reporter and surfaces.
Publish readiness without waiting for individual remote conversations.

Preserve authenticated ownership and duplicate-process guards. Restoration
itself sends no instruction and does not release queued instructions. Historical
outcome uncertainty can remain after restoring access to a conversation.
Independent eligible conversations use bounded concurrent restoration with
focused-session priority and per-session deduplication.

## Consequences

- The application no longer hides global preparation behind a usable task screen.
- Startup and normal runtime workers need explicit, cancellable start/stop ownership.
- Preparation duration remains real startup latency; timing visibility alone is
  not a performance improvement. Outstanding-state selection addresses repeated work.
- Future required housekeeping must join this gate and declare its startup step.
- Remote uncertainty remains session-scoped and cannot authorize a replacement.

The [requirement](../specs/platform/requirements/startup-housekeeping.md) and
[design](../specs/platform/system-design/startup-housekeeping.md) define the
implementation contract. The implementation was authorized after the design
handoff. Bounded local survivor ownership discovery precedes new local launches;
remote attachment and native conversation load remain outside the readiness gate.

## Alternatives Considered

- Move all recovery before readiness: a disconnected remote executor could hold
  the whole installation unavailable, and native conversation load is not global
  housekeeping.
- Add timing labels while retaining current ordering: this leaves global work
  competing with agent restoration and preserves recurring historical scans.
- Enable local process survival by default: this changes executor lifetime policy
  and does not remove expensive database work.
- Introduce a global applied-migration ledger: unnecessary for restricting the
  identified replay queries to outstanding state and much broader than this change.
