---
created: 2026-09-25
status: done
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
legacy_specs: []
---

# Implementation Plan: Keep live custom-agent discovery coherent

Keep discovery results consistent with the operator-managed agent registry during
the full lifetime of the backend. Registry changes must invalidate cached
results, and an older sweep must not publish data after that invalidation.

The existing agent requirement and system design define the live-registry
contract. This plan records the implementation and its rollback regression.

## Work orders

- [Task 01: Keep custom-agent discovery cache coherent](task-01-discovery-cache-coherence.md)

## Verification

The work order records the focused discovery and settings-controller tests.
