---
id: "02-historical-window"
title: "Bound historical timeline rendering"
status: completed
wave: 2
depends_on: ["01-working-tree-window"]
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.2
  - AC-UI-BOUNDED-CHANGES-001.3
  - AC-UI-BOUNDED-CHANGES-001.4
  - AC-UI-BOUNDED-CHANGES-001.7
  - AC-UI-BOUNDED-CHANGES-001.9
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 02: Bound historical timeline rendering

## Summary

Bring commit headers, inline commit files, and provider file rows into the same
bounded timeline. Preserve lazy loading and historical source identity.

## In scope

- Lift commit and provider-group expansion into timeline ownership.
- Add descriptors for historical children and loading/error/empty states.
- Replace hidden inline child trees with metadata and virtual rows.
- Add scoped request ownership and bounded collapsed metadata reuse.
- Preserve complete local/provider targets and existing request restrictions.

## Out of scope

Standalone commit diff/index virtualization, provider pagination, and global
query-cache changes remain outside this repair.

## Acceptance

1. Commit headers, provider files, and expanded historical files share the panel-wide row budget. Collapse mounts no descendants.
2. Explicit expansion loads once per current target. Scroll does not refetch. Recent reopen reuses data, and eviction permits a normal reload.
3. Local and GitHub file actions open the correct historical target. Late completions cannot cross context or target changes.

## ASCII UI preview

UI-03, from the [combined preview](plan.md#ascii-ui-preview):

```text
Commits (50000)
v abc123 message             [Open]
    v src/
      historical.ts          +4 -2
> def456 message             [Open]
```

Phone uses wrapped names and touch controls in its existing Changes surface.
Loading/error/retry replaces only the child status row. Covers .2, .3, .4,
.7, and .9. No historical row gains Stage or Discard.

## Verification

First record failing cases `bounds expanded historical files` and
`does not fetch details on virtual remount`. Keep full target/source assertions.

From the repository root:

```bash
cd apps/web
pnpm exec vitest run components/task/changes-timeline-model.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-inline-commit-state.test.ts components/task/commit-row.test.tsx components/task/commit-row-files.test.tsx components/task/changes-panel-pr-files.test.tsx components/task/commit-detail-panel.test.tsx components/task/commit-detail-request.test.ts
pnpm run typecheck
pnpm e2e:run --project chromium tests/git/commit-file-navigation.spec.ts tests/git/changes-panel-section-order.spec.ts
pnpm e2e:run --project mobile-chrome tests/git/mobile-commit-file-navigation.spec.ts
```

Add controller tests for first expansion, concurrent expand, retry, cached reopen,
eviction, collapse during read, and A-to-B-to-A stale completion. Include equal
SHAs across repositories and a GitHub failure with no local fallback.

## Files likely touched

- `apps/web/components/task/changes-timeline-model.ts` and viewport adapters.
- New `apps/web/components/task/changes-inline-commit-state.ts` and its tests.
- `apps/web/components/task/commit-row.tsx`, `commit-row-files.tsx`.
- `apps/web/components/task/changes-panel-pr-files.tsx`, `changes-panel-repo-groups.tsx`.
- `apps/web/components/task/changes-panel-timeline.tsx`, `changes-panel-body.tsx`.
- `apps/web/hooks/domains/session/use-commit-detail.ts` only for a safe shared extraction.
- Existing component and browser tests named above.

## Risks

The existing detail hook is row-local. Remounting it directly would repeat
requests and discard expansion. Patch bodies must not enter the inline cache.

## Dependencies

01-working-tree-window.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/bounded-changes-rendering.md).
- [System design](../../specs/ui/system-design/bounded-changes-rendering.md).
- [Plan](plan.md), including evidence, exclusions, and browser matrix.
- Existing Files virtualizer and Changes/commit tests named in this work order.

## Results

Completed on 2026-09-30.

- RED: the first controller test run failed because the history detail owner did
  not exist. The first desktop browser run also exposed an old locator that
  assumed inline files remained descendants of `CommitRow`; the test now scopes
  files through the commit's `aria-controls` owner, matching separate virtual
  rows.
- GREEN: one panel-owned TanStack viewport now includes working files, PR
  files, commit headers, and expanded commit file metadata. Commit details load
  only on explicit expansion, coalesce by complete source target, strip patches,
  fence task/session/environment changes, and retain a collapsed LRU bounded to
  eight targets and 50,000 file descriptors. Local and GitHub detail actions
  retain their source targets.
- `pnpm exec vitest run components/task/changes-timeline-model.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-inline-commit-state.test.ts components/task/commit-row.test.tsx components/task/commit-row-files.test.tsx components/task/changes-panel-pr-files.test.tsx components/task/commit-detail-panel.test.tsx components/task/commit-detail-request.test.ts`: 8 files, 46 tests passed.
- `pnpm run typecheck`: passed after integrating the history rows and controller.
- `pnpm e2e:run --project chromium tests/git/commit-file-navigation.spec.ts tests/git/changes-panel-section-order.spec.ts`: 7 passed. The managed run built the production Vite bundle.
- `pnpm e2e:run --project mobile-chrome tests/git/mobile-commit-file-navigation.spec.ts`: 1 passed. The managed run built the production Vite bundle.
- Controller tests cover initial detail loading, concurrent coalescing, cached
  reopen, target and file-count eviction, collapse during a request, explicit
  retry, equal SHAs across repositories, and A-to-B-to-A stale completion.

Task 03 remains responsible for the whole-panel stress, focus, anchor, and
measurement lifecycle coverage.
