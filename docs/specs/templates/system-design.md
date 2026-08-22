---
status: draft
system: <system-slug>
requirements:
  - REQ-<SYSTEM>-<CAPABILITY>-001
---

# <Capability> System Design

## Purpose and boundaries

Explain the technical responsibility and its boundaries.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-<SYSTEM>-<CAPABILITY>-001` | [Purpose and boundaries](#purpose-and-boundaries) |

## Components and responsibilities

Name the runtime components and their responsibilities.

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
