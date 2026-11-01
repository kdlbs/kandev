---
status: active
system: executors
created: 2026-10-03
owners:
  - kandev
---

# Agent Runtime Footprint Requirements

## Scope

This delivery carries the existing read-only observation requirement from the
runtime-reclamation initiative. Terminal teardown, startup recovery filtering,
automatic stopping and retention policy remain separate; this slice changes none.

## Requirements

### REQ-EXECUTORS-RUNTIME-RECLAIM-003: Authoritative per-session footprint

An operator shall be able to attribute live agent cost to the session that holds
it, on both desktop and phone.

The projection shall report, per live runtime: the owning session and task
identity, the runtime state, the number of owned live processes, committed bytes,
and resident bytes. Attribution shall be by instance and session identity, never
by process name or a bare process identifier.

A measurement that cannot read every owned process shall report the count it
could not read, and its byte totals shall then be read as a lower bound. A
platform that cannot report commit charge per process shall report zero committed
bytes rather than estimating from resident bytes.

The projection shall carry no transcript content, credential, resume token, or
command line.

Acceptance criteria:

- **AC-EXECUTORS-RUNTIME-RECLAIM-003.1:** Every projected row carries session identity, runtime state, owned process count, committed bytes, and resident bytes.
- **AC-EXECUTORS-RUNTIME-RECLAIM-003.2:** Every measured process is a descendant of the owning instance's own agent process, never a host-wide name match.
- **AC-EXECUTORS-RUNTIME-RECLAIM-003.3:** An owned process that cannot be measured is counted as unreadable, and a nonzero unreadable count is presented as making the byte totals a lower bound.
- **AC-EXECUTORS-RUNTIME-RECLAIM-003.4:** A platform that cannot report commit charge per process reports zero committed bytes rather than an estimate.
- **AC-EXECUTORS-RUNTIME-RECLAIM-003.5:** No task, session, execution, user, or agent identifier appears as an expvar label.
- **AC-EXECUTORS-RUNTIME-RECLAIM-003.6:** The phone view presents the same capability as the desktop view.

## Traceability

| Requirement | Work order |
| --- | --- |
| REQ-EXECUTORS-RUNTIME-RECLAIM-003 | [Footprint projection](../../../plans/agent-runtime-reclamation/task-02-runtime-footprint-projection.md) |
