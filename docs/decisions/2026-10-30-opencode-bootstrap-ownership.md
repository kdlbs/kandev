# ADR-2026-10-30-opencode-bootstrap-ownership: Destination-owned OpenCode bootstrap

**Status:** accepted
**Date:** 2026-10-30
**Area:** backend, protocol

## Context

OpenCode V2 import skips existing credential IDs and preserves destination
selections. Exporting controller credentials on every restart introduces a
controller-auth dependency without repairing revoked destination credentials.
The operator explicitly chose controller-independent authenticated restarts.
CLI import is not one atomic multi-account transaction, so account presence
alone cannot prove a previously interrupted bootstrap completed.

## Decision

Treat V2 credential transfer as bootstrap, not synchronization. Saved destination
accounts remain session-owned. Inspect them locally and skip source export for
a completed or pre-existing store. Track incomplete bootstrap with secret-free,
store-scoped pending state so interrupted imports require merge retry. Never
silently overwrite credentials or switch accounts on a controller change.
Reauthentication is an explicit session-local operation using the supported CLI.

This decision is specific to V2 and does not change the V1 file-merge policy in
[the existing decision](2026-09-05-agent-owned-credential-file-conflicts.md).
Runtime identity follows [OpenCode adoption](2026-09-27-opencode-runtime-adoption.md).

## Consequences

Completed sessions can restart without controller authentication. Presence is
not a provider health check: revoked or wrong-provider accounts require explicit
recovery. Pending imports require the controller until a successful retry.
The session-state marker contains no credentials and cannot replace local
credential inspection. Concurrent OAuth refresh remains a live validation gate.

## Alternatives Considered

- Export every restart: couples availability to controller authentication and
  cannot replace existing IDs anyway.
- Overwrite destination accounts: can discard refreshed tokens and selections.
- Skip whenever any account exists: mistakes partial import for completion.
- Credential broker: changes ownership and deployment architecture without
  evidence that the CLI-based bootstrap boundary is insufficient.

## Related contracts

- [Requirements](../specs/executors/requirements/opencode-v2-credentials.md)
- [System design](../specs/executors/system-design/opencode-v2-credentials.md)
