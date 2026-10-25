# ADR-2026-09-05-bounded-clarification-response-path: Bound clarification responses with indexed lookup

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend, frontend, protocol

## Context

Clarification responses start from a pending ID in the route. The repository
therefore finds the bundle before it knows the session ID.

The existing index starts with the session ID. SQLite cannot use that index for
a pending-ID-only query. A database with about 904,000 messages used a full
table scan. Answer and Skip requests then took 37 to 55 seconds.

The backend bounded only the atomic claim. The identity query before the claim
used the request context without an internal deadline. The web client also had
no deadline.

SQLite and PostgreSQL use the same repository. Their JSON expressions differ,
and both database paths must keep the same response contract.

## Decision

Add a second partial expression index for message pending IDs. The pending-ID
expression is the first key. The ordering keys are `created_at` and `id`.

Keep the existing session-first index. It serves a different query shape. Use a
new index name so existing databases create the new index at startup.

Use the current replay-safe startup migration path for SQLite and PostgreSQL.
Use regular index creation for both drivers. Kandev supports one backend
replica and stop-the-writer upgrades.

Give all pre-claim work one five-second internal deadline. Return HTTP 503 with
code `temporarily_unavailable` when this deadline expires.

Give the web request a 40-second deadline. Preserve the entered answer after an
error. Retry through the existing durable claim and winner-reconstruction
path.

## Consequences

Pending-ID response work does not scan unrelated message history. The first
startup after an upgrade must build one partial index.

Large databases can have a longer first startup and need temporary disk space.
An index-creation error stops startup instead of enabling the slow access path.

The 503 code becomes part of the clarification response contract. Clients can
show a retryable result without treating it as an expired question.

The browser always leaves its progress state. A client timeout can still be an
ambiguous transport result. Durable winner reconstruction makes a retry safe
and prevents duplicate delivery.

PostgreSQL does not get concurrent index management. A future multi-replica
design must supersede this decision before it changes the migration model.

## Alternatives Considered

### Reorder the existing index

This option can regress session-scoped queries. A new index has a smaller and
reviewable effect.

### Add a clarification table

This option removes JSON expression queries. It also requires row migration,
dual-write rules, and more recovery changes than this fault requires.

### Add only a browser deadline

This option stops the spinner but keeps the database scan. It also creates more
ambiguous responses under normal load.

### Use concurrent PostgreSQL index creation

This option reduces write locks for a live multi-replica service. Kandev does
not support that deployment. It also needs invalid-index recovery and migration
coordination outside the current startup model.
