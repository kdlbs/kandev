# ADR-2026-09-05-host-mediated-execution-leases: Host-mediated execution leases

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend, security, protocol

## Context

Coordinator messages are peer input, not user authorization. Treating a
message as trusted user input would weaken the executor trust boundary, but a
Coordinator needs a narrow route to request bounded repository recovery.

## Decision

An operator may grant `execute` to an authenticated workspace Coordinator
principal. It authorizes only a Host to create and consume a short-lived
execution lease. The Host derives and binds the lease to grant, principal,
workspace, target task, repository, branch, expected Git object id, and one
fixed operation: fast-forward push, index update, or fast-forward merge.

The Host rechecks active grant and principal before issuance and consumption,
persists an opaque one-time receipt, and records the result in the coordinator
audit trail. A lease is denied on any scope, expiry, revocation, workspace,
repository, branch, head, or operation mismatch. Agents receive neither
credentials nor a reusable authorization token.

The executor transport is a dependent Host delivery. It must consume the
server-side receipt atomically and may not treat prompt text, a command prefix,
or serialized agent input as a lease.

## Consequences

The Settings and API grant surface can request and revoke `execute` while
preserving the established principal and audit model. The shared contract
supplies a deny-by-default scope validator without exposing paths or commands.

This does not authorize deletion, force-push, rebase, history rewrite,
arbitrary shell execution, raw filesystem access, cross-workspace access, or
credential disclosure. The executor delivery needs its own review.

## Alternatives Considered

Promoting Coordinator messages to user input was rejected because a peer agent
could cross an authorization boundary. Agent-visible signed tokens were
rejected because they are replayable and can leak. A command-prefix bypass was
rejected because it cannot bind an action to durable task and repository ids.
