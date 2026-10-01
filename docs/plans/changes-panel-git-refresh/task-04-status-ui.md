---
id: "04-status-ui"
title: "Present truthful desktop and mobile states"
status: done
wave: 4
depends_on: ["03-refresh-delivery"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.23
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.24
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.30
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.32
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 04: Present truthful desktop and mobile states

## Summary

Keep accepted membership and refresh state separate. Connect finite refresh/recovery to the existing activation paths and render the same domain states on both surfaces.

## In scope

- Add the small environment/repository refresh-state map. Accept ordering/quality before clearing data or derived caches.
- Coalesce foreground requests and finite recovery across sibling consumers. Cancel timers/waiters on scope changes and disconnect. Preserve existing focus reference counts.
- Before UI changes, add the two focused browser suites from Task 05 and record a red result for missing/failed status rendering.
- Add shared loading/unavailable/last-observed notices, localized Retry, pending row statistics, and selected-diff placeholders.
- Fix auto-close and Review/editor projection based on membership/layer readiness. Pending content cannot clear models or review hashes.
- Audit all pending-stat consumers, including upstream action policy, task badges, signatures, and cumulative-diff invalidation.
- Complete en, pt-pt, zh-cn, zh-hk, zh-tw, and ja catalogs. Generate Traditional Chinese and pseudo locales with existing scripts.

## Out of scope

Tracker scheduling and source selection redesign. No independent phone data model.

## Acceptance

- Pure/store/hook tests distinguish missing, summary-only, clean, pending, failed, and prior-good states. Out-of-order responses cannot overwrite newer scope/revisions.
- Pending files/facets remain selectable and keep their diff surface open. Clean membership alone can remove an absent target, with repository/layer isolation.
- Existing activation paths drive at most one finite recovery attempt per scope. Desktop/mobile share data semantics and accessible localized retry states.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-01: Changes status (desktop)

Entry: task Changes panel. The existing toolbar remains above the single scroll body.

```text
Changes                                      [Refresh]
Loading:       Checking changed files...
Unavailable:   Git status unavailable.        [Retry]
Prior data:    Refresh failed. Showing last observed changes. [Retry]
Dirty:         Changed files are ready. Diffs are loading.
  Unstaged
    src/a.ts   Modified                       Diff pending
  Staged
    src/b.ts   Modified                       Diff pending
Clean:         Your changed files will appear here
```

### UI-02: Changes status (phone)

Entry: task bottom navigation > Changes. Reuse the shipped mobile Changes body and full-height diff drawer.

```text
< Task                 Changes
Git status unavailable.
[ Retry (touch target) ]
Unstaged
  src/a.ts  Modified
  Diff pending
-----------------------------
[Sessions] [Files] [Terminal] [Changes]

< Back        src/a.ts
Diff is loading...
```

The status row scrolls with the existing body. Phone navigation and drawer controls retain their safe-area behavior.
Retry is 44px on touch surfaces and 28px on desktop. Each surface has one vertical scroll owner.
The clean line appears only after complete successful empty membership, with no independent PR or commit content.
Row selection opens a pending placeholder and remains open until authoritative membership removes that repository/path/layer.
These structures satisfy criteria `.23` through `.30`. Copy and spacing are illustrative and must use localization and existing primitives.

## Verification

Run from repository root. Use `/tdd` and run each new regression red before implementation.
If `apps/node_modules` is absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-git-derived.test.ts hooks/domains/session/use-session-git-status.test.tsx hooks/domains/session/use-review-sources.test.ts hooks/domains/session/use-review-sources-facets.test.ts hooks/domains/session/use-review-sources-scoped.test.ts hooks/domains/session/use-review-sources-normalization.test.ts lib/state/slices/session-runtime/git-status-state.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts lib/state/slices/session-runtime/set-git-status-return.test.ts lib/ws/handlers/git-status.test.ts components/task/task-changes-panel.test.ts components/task/changes-panel-focus.test.ts components/task/mobile/mobile-changes-panel.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-panel-refresh-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-changes-panel-refresh-recovery.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/git/changes-panel-refresh-recovery.spec.ts`
- `apps/web/e2e/tests/git/mobile-changes-panel-refresh-recovery.spec.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-state.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-state.test.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-normalizer.ts`
- `apps/web/lib/ws/handlers/git-status.ts`
- `apps/web/lib/ws/client.ts`
- `apps/web/hooks/domains/session/use-session-git-status.ts`
- `apps/web/hooks/domains/session/use-session-git.ts`
- `apps/web/hooks/domains/session/use-session-git-derived.ts`
- `apps/web/hooks/domains/session/use-review-sources.ts`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-body.tsx`
- `apps/web/components/task/task-changes-panel-state.ts`
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/task/mobile/mobile-diff-sheet.tsx`
- `apps/web/components/review/types.ts`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`
- `apps/web/src/locales/ja/task.json`

## Dependencies

Complete Task 03 first. Read its Results before changing the shared contract.

## Risks

- Partial status is complete membership but incomplete details. Do not confuse omission of diff data with omission of a file.
- Existing non-refresh pending operation ownership remains independent of refresh settle events.
- StrictMode and multiple hooks can duplicate triggers unless coalescing happens before asynchronous state updates.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md).
- [System design](../../specs/platform/system-design/workspace-git-status.md).
- [Plan](plan.md), accepted ADR, scoped `AGENTS.md`, and existing tests beside owned code.

## Results

The Changes panel now distinguishes membership readiness from diff enrichment, retains prior accepted rows through refresh failure, and shows localized Retry only for a failed status request. Pending files remain selectable in Review and Changes; auto-close and review progress use repository/path/layer membership rather than diff-string presence. Desktop and mobile use the same status model, with a localized placeholder in the selected diff surface.

The desktop and mobile recovery regressions were red before the status presentation work. After implementation:

- Focused Git status, Review projection, Changes body, and mobile Vitest selection: 19 files, 188 tests passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps && pnpm --filter @kandev/web lint)`: passed with zero warnings.
- `pnpm run i18n:zh-hant` and `pnpm run i18n:pseudo`: generated the derived catalogs.
- `(cd apps/web && pnpm run i18n:check)` and `(cd apps/web && pnpm run i18n:ratchet)`: passed.
- The controlled desktop and mobile browser flows passed; mobile full-height drawer bounds and screenshot review are recorded in Task 05.

Task 04 is done. Final integrated documentation validation and `git diff --check` remain in Task 05.
