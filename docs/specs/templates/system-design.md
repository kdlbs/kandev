---
status: draft
system: <system-slug>
requirements:
  - REQ-<SYSTEM>-<CAPABILITY>-001
---

# <Capability> System Design

## Purpose and boundaries

Explain why this system owns the technical contract. Name adjacent contracts
that this design uses but does not own.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-<SYSTEM>-<CAPABILITY>-001` | [Purpose and boundaries](#purpose-and-boundaries) |

## Components and responsibilities

Name all stable runtime components and their responsibilities. Cover backend
and frontend components when both implement the same owned outcome.

## Data and contracts

Define the stable models, interfaces, events, APIs, and configuration.

## Control flow

Describe the interaction direction and the data that crosses each boundary.

## Failure and recovery

Describe retries, degraded behavior, user-visible errors, and safe failure.

## Persistence

Describe storage ownership, transactions, migrations, retention, and restart
behavior.

## Security

Describe permissions, trust boundaries, and sensitive data handling.

## Observability

Describe the logs, metrics, traces, and diagnostics that expose behavior.

## Related decisions

- [ADR](../../../decisions/<adr-id>.md)
