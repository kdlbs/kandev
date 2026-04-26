# Orchestrate: Master Implementation Plan

**Date:** 2026-04-26
**Status:** proposed
**Specs:** all `docs/specs/orchestrate-*` specs (10 specs)
**Plans:** Waves 1-7 in `docs/plans/2026-04-orchestrate-wave*.md`

## Overview

Orchestrate is an autonomous agent management layer built on top of kandev's existing task/session/workflow system. Implementation is split into 7 waves with clear dependency ordering. Each wave contains parallelizable work units that can be assigned to subagents.

## Wave Dependency Graph

```
Wave 1: Shell & Data Model          (foundation, no deps)
    |
Wave 2: Core CRUD                   (depends on Wave 1)
    |   ├── 2A: Agent Instances     (parallel)
    |   ├── 2B: Skill Registry      (parallel)
    |   └── 2C: Projects            (parallel)
    |
Wave 3: Task System & Issues UI     (depends on Wave 2)
    |   ├── 3A: Task Model Extensions (do first)
    |   ├── 3B: Issues List Page    (parallel, after 3A)
    |   ├── 3C: Task Detail Pages   (parallel, after 3A)
    |   └── 3D: New Issue Dialog    (parallel, after 3A)
    |
Wave 4: Orchestration Engine        (depends on Wave 3)
    |   ├── 4A: Wakeup Queue        (do first)
    |   ├── 4B: Scheduler Extension (after 4A)
    |   └── 4C: Prompt Builder      (parallel with 4B)
    |
Wave 5: Monitoring & Governance     (depends on Wave 3, partially Wave 4)
    |   ├── 5A: Cost Tracking       (parallel)
    |   ├── 5B: Budget Policies     (parallel with 5A)
    |   ├── 5C: Activity Log        (parallel)
    |   ├── 5D: Inbox & Approvals   (after 5A-5C)
    |   └── 5E: Dashboard           (after 5D)
    |
Wave 6: Automation                  (depends on Wave 4, Wave 5)
    |   ├── 6A: Routines            (parallel)
    |   ├── 6B: Org Chart Page      (parallel)
    |   └── 6C: Notification Wiring (parallel)
    |
Wave 7: Extended Features           (depends on Wave 5, Wave 6)
        ├── 7A: Channels            (parallel)
        ├── 7B: Agent Memory        (parallel)
        ├── 7C: Self-Improvement    (after 7B)
        └── 7D: Config Export/Import(parallel)
```

## Execution Strategy

### Subagent Assignment

Each wave's parallel items run as separate subagents in isolated worktrees. Sequential items within a wave run in the same subagent.

**Wave 1** -- 1 subagent (foundation must be single, coherent):
- Backend: all DB tables, package structure, API stubs, event types
- Frontend: page shell, routing, layout, sidebar, store stub, API client stubs, homepage link

**Wave 2** -- 3 subagents in parallel:
- Subagent 2A: Agent instances (backend CRUD + frontend pages)
- Subagent 2B: Skill registry (backend CRUD + materialization + frontend pages)
- Subagent 2C: Projects (backend CRUD + frontend pages)

**Wave 3** -- sequential 3A, then 3 subagents in parallel:
- Subagent 3A: Task model extensions (backend only, must complete first)
- Subagent 3B: Issues list page (frontend, after 3A)
- Subagent 3C: Task detail pages (frontend, after 3A)
- Subagent 3D: New issue dialog (frontend, after 3A)

**Wave 4** -- sequential 4A, then 2 subagents:
- Subagent 4A: Wakeup queue service (backend, must complete first)
- Subagent 4B: Scheduler extension + skill injection (backend, after 4A)
- Subagent 4C: Prompt builder (backend, parallel with 4B)

**Wave 5** -- 3 subagents parallel, then 2 sequential:
- Subagent 5A: Cost tracking + models.dev integration (backend + frontend)
- Subagent 5B: Budget policies + enforcement (backend + frontend)
- Subagent 5C: Activity log (backend + frontend)
- Subagent 5D: Inbox + approvals + execution policy (backend + frontend, after 5A-5C)
- Subagent 5E: Dashboard page (frontend, after 5D)

**Wave 6** -- 3 subagents in parallel:
- Subagent 6A: Routines (backend + frontend)
- Subagent 6B: Org chart page (frontend only)
- Subagent 6C: Notification wiring (backend, small)

**Wave 7** -- 3 subagents in parallel, then 1:
- Subagent 7A: Channels (backend + frontend)
- Subagent 7B: Agent memory (backend + frontend)
- Subagent 7D: Config export/import (backend + frontend)
- Subagent 7C: Agent self-improvement (backend, after 7B, small)

### Per-Subagent Instructions

Each subagent receives:
1. The relevant wave plan document
2. The relevant spec document(s)
3. The CLAUDE.md project instructions
4. Instruction to write unit tests for all new code
5. Instruction to run `make fmt` then `make typecheck test lint` before completing
6. Instruction to NOT modify files outside its scope (avoid merge conflicts between parallel agents)

### Merge Strategy

- Wave 1 merges to the feature branch first (foundation)
- Within a wave: parallel subagents work in isolated worktrees, merged sequentially after all complete
- Merge order within waves: backend-only subagents first, then frontend subagents
- After each wave merges: run full `make -C apps/backend test` and `cd apps && pnpm --filter @kandev/web typecheck` to verify integration

## Testing Strategy

**Unit tests** (each wave):
- Backend: `*_test.go` alongside source, using standard `testing` package
- Frontend: `*.test.ts` using Vitest + React Testing Library
- Run as part of each subagent's verification step

**E2E tests** (after all waves, user validation):
- Written after user tests the full feature and provides feedback
- Playwright-based, using existing E2E patterns
- Cover golden paths: create agent -> assign task -> agent runs -> reviews -> done
- Cover cross-project delegation flow
- Cover routine triggering
- Cover inbox approval flow

## Key Files Modified (cross-wave)

These files are touched by multiple waves and need careful merge ordering:

| File | Waves | Notes |
|------|-------|-------|
| `cmd/kandev/gateway.go` | 1, 4, 5, 6, 7 | Route registration (additive, low conflict risk) |
| `cmd/kandev/services.go` | 1, 4, 5, 6, 7 | DI wiring (additive) |
| `internal/events/types.go` | 1 | All event types added in Wave 1 |
| `internal/task/models/models.go` | 3 | Task model extensions |
| `internal/task/repository/sqlite/base.go` | 1, 3 | Schema migrations |
| `apps/web/lib/state/slices/orchestrate/` | 1, 2, 3, 5 | Store slice (additive) |
| `apps/web/lib/api/domains/orchestrate-api.ts` | 1, 2, 3, 5, 6, 7 | API client (additive) |
| `apps/web/app/orchestrate/layout.tsx` | 1 | Layout (set once in Wave 1) |
| `apps/web/app/orchestrate/components/orchestrate-sidebar.tsx` | 1, 2, 5, 6 | Sidebar (additive) |

## Estimated Scope

| Wave | Backend files | Frontend files | Tests | Parallel subagents |
|------|--------------|----------------|-------|-------------------|
| 1 | ~15 | ~20 | ~10 | 1 |
| 2 | ~15 | ~25 | ~15 | 3 |
| 3 | ~8 | ~20 | ~12 | 4 (1 seq + 3 par) |
| 4 | ~10 | ~2 | ~15 | 3 (1 seq + 2 par) |
| 5 | ~15 | ~15 | ~20 | 5 (3 par + 2 seq) |
| 6 | ~10 | ~10 | ~12 | 3 |
| 7 | ~15 | ~12 | ~15 | 4 (3 par + 1 seq) |

## Critical Path

Wave 1 -> Wave 3A -> Wave 4A -> Wave 4B is the critical path. Everything else can be parallelized around it. Getting the data model, task extensions, and wakeup queue working unlocks the most downstream work.

## Guardrails

- **No breaking changes**: all work is additive. Existing kanban board, task detail, workflows, sessions continue to work unchanged.
- **Feature flag**: Orchestrate UI is behind the `/orchestrate` route. Users who don't navigate there see no changes.
- **Shared models**: Task model extensions are additive columns with defaults. Orchestrate tasks use a system workflow (workflow_id stays NOT NULL). Existing tasks are unaffected.
- **Isolated package**: `internal/orchestrate/` is a new package. No modifications to existing service/repository code except additive task model fields and event type constants.
