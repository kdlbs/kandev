---
spec: docs/specs/tasks/requirements/portable-workspace-adoption.md
created: 2026-10-11
status: in_progress
---

# Portable workspace adoption implementation

## Overview

Approved Kandev design 531dfb46 and reproduced gate b8b95a39 establish the distinct
core adoption gap. Preserve unrelated guarded-ACP commit cfd6bdf65a under
`preserve/portable-adoption-guarded-acp-cfd6bdf65a`. Task branch base is upstream
00b5d599968a713a7c24318a17128a6b4b1dc431. Isolated probes remain local.

## Ordered tasks

1. [Inspection](task-01-inspection.md): pinned same-root filesystem and origin proof.
2. [Publication](task-02-publication.md): claim, selection CAS, atomic receipt, replay.
3. [Operator API](task-03-api.md): service authorization, routes and resume identity.
4. [Verification](task-04-verification.md): regression, documentation, commit and side sessions.

MAIN alone writes source. Independent Kandev Review and QA inspect the frozen
committed candidate. No PR or deployment work is included in this continuation.

## Tests

Real Git, SQLite and env-gated PostgreSQL fixtures cover HTTP preview/apply,
identity/content preservation, old marker provenance, stale sessions/inventory,
foreign paths/origins/branches, active consumers, rollback and operation replay.
Use named TestWorkspaceAdoption tests in affected worktree, task handlers and
repository packages; run with race for final behavior checks. Test logs and probe
failures are local evidence under .kandev/portable-adoption/artifacts.

## Verification Results

Step 1 isolated gate passed against current main 1bba2c18 in the previous turn.
Implementation tasks 1-3 are complete. MAIN targeted race checks passed across
worktree, task handlers, repository and backend composition. SQLite and task-owned
PostgreSQL 18.6 publication, rollback, schema replay and exclusive claim checks
passed. Independent Review and QA remain pending for the frozen candidate.
No submodules or live deployment involved. No PR or overlay retirement claim.
