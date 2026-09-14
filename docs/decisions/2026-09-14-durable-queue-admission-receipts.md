# ADR-2026-09-14-durable-queue-admission-receipts: Durable queue admission receipts

**Status:** proposed
**Date:** 2026-09-14
**Area:** protocol

## Context

Ordinary queue submissions lack server replay protection. Queue rows can merge or disappear before a client receives their admission response.
Plan-comment rows have special replay handling and intentionally avoid automatic merging. Applying that exception to every prompt changes existing merge behavior.

## Decision

The proposed fix stores an admission receipt atomically with each identified ordinary admission.
The receipt binds the request fingerprint and accepted response to the task, session incarnation, and client queue ID.
It survives queue operations until the owning session is deleted. Stale incarnation checks still reject old requests.

Receipt identity is separate from queue-row identity and merge metadata. Retries consult the receipt before repeating capacity checks or attachment claims.
The client enables ordinary retries only after the server supplies this contract.

## Consequences

Lost responses no longer require an existing queue row or transcript message to prevent duplicate admission.
The repository needs additive schema initialization, database parity, and lifecycle cleanup coverage.
Receipt storage grows with accepted submissions in retained sessions. Response snapshots must not copy inline attachment bytes.
This proposal accompanies a draft fix package. It does not authorize implementation.

## Alternatives Considered

- A longer timeout reduces failures but does not settle lost responses.
- Searching queue and transcript alone leaves gaps during dispatch and after removal or merging.
- Reusing the plan-comment path disables ordinary automatic merging and requires nonexistent comment references.
- An in-memory receipt loses replay protection after restart.

## Related design

- [Queue admission](../specs/tasks/system-design/queue-admission.md)
