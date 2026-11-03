# ADR-2026-10-11-core-bounded-queue-recovery: Core queue observations
**Status:** accepted
**Date:** 2026-10-11
**Area:** backend, protocol

## Context
Replacement sessions and parents need immutable observations of undelivered instructions. Local 360ea9139/75efb8ec3 supply readers absent upstream. Docker-hosted server deployment is independent of task executors.

## Decision
Core messagequeue owns bounded nonmutating observations and hashes. Authenticated task MCP handlers own exact own-terminal/direct-child authority; server owns schemas and forwarding. Reuse #3377 census without duplicating registration. Plugins may own recovery policy but cannot read database files or widen authority. Source Coordinator owns PR reconciliation; deployment owner verifies release parity and removes overlays.

[Task requirements](../specs/tasks/requirements/bounded-queue-recovery.md) and [design](../specs/tasks/system-design/bounded-queue-recovery.md) own behavior.

## Consequences
Readers are portable across executors/databases. Limits can reject large inventories. Snapshot changes require restarting pagination. Existing Repository.Snapshot writes status generations and is unsuitable. Empty FIFO does not prove dispatch absence or replacement acknowledgement.

## Alternatives Considered
Plugin-only needs the same core API or unsafe schema-coupled access. Deployment-only overlays supply local behavior without portable source/release acceptance.
