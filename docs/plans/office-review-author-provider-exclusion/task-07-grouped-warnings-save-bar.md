---
id: "07-grouped-warnings-save-bar"
title: "Grouped exclusion warnings and the routing page save bar"
status: done
wave: 6
depends_on:
  - "06-routing-control-web"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.10
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.7
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 07: Grouped exclusion warnings and the routing page save bar

## Summary

Show warnings that share a cause and fix as one warning with a list of
affected steps, on the routing banner, the import dialog, and workflow sync.
Replace the routing page's bottom Save button with the shared floating
settings save bar.

## Scope

1. Backend: `syncWarningTexts` groups sync warnings by code, provider, and
   agent.
2. Web: `groupAuthorExclusionWarnings`; `AuthorExclusionWarningList` renders
   groups (problem, step list with a "Show all" toggle past three, fix with
   `count` plurals); count-agnostic problem copy; locales.
3. Web: `SettingsSaveProvider` on the routing route and one save contributor
   for the page; bottom Save removed.
4. E2E: desktop and phone setup specs use the save bar and check grouping.

## Exclusions

Workflow-editor control (issue #5347). Other Office pages' save behavior.

## UI-05 (from [plan](plan.md#ui-05-provider-routing-settings-floating-save-bar))

```text
|        | Unsaved changes            [Discard]  [Save]  |        |
```

## Implementation acceptance

- Six steps with the same single-provider warning render as one warning with
  three listed steps and a "Show all 6 steps" toggle; the sync result carries
  one string for them.
- Changing a routing setting or a switch shows the floating save bar; Save
  applies both writes, Discard restores the stored values.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/workflow/service/...
cd ../web
pnpm exec vitest run --maxWorkers=1 lib/author-exclusion-warning-groups.test.ts \
  components/author-exclusion-warning-list.test.tsx app/office/workspace/routing
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run typecheck
pnpm e2e:run --host --shards 1 --project chromium tests/office/office-review-author-exclusion-setup.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-review-author-exclusion-setup.spec.ts
```

## Likely files

- `apps/backend/internal/workflow/service/import_exclusion_advisor.go`, `sync_apply.go` (+ test)
- `apps/web/lib/author-exclusion-warning-groups.ts` (+ test)
- `apps/web/components/author-exclusion-warning-list.tsx` (+ test)
- `apps/web/app/office/workspace/routing/page.tsx`, `use-routing-save-contributor.ts` (+ test)
- `apps/web/src/office-routes.tsx`
- `apps/web/src/locales/*/workflows.json`
- `apps/web/e2e/tests/office/*review-author-exclusion-setup.spec.ts`

## Risks

- The save bar's `content` placement needs a positioned ancestor; reuse the
  Office settings route's mounting.
- The save must rethrow on failure, or the bar reports success after a
  failed write.

## Results

Implemented with TDD.

- `syncWarningTexts` returns one sync warning per code, provider, and agent,
  listing the affected steps and using "this step" or "these steps".
- `groupAuthorExclusionWarnings` and the rewritten `AuthorExclusionWarningList`
  show each group's problem once, its steps (three, then a "Show all N steps"
  toggle), and one plural-aware fix. The single-provider problem sentence no
  longer says "the reviewer", so it reads correctly for approver steps too.
  Locales: all seven catalogs plus pseudo; Chinese, Japanese, and Korean use
  count-neutral "affected steps" wording because their plural rules only have
  the `other` form.
- The routing route mounts `SettingsSaveProvider`; `useRoutingSave` registers
  one `office-routing` contributor (routing write, then switch write; failures
  rethrown; Reset restores both drafts). The bottom Save button is gone.
- E2E: the desktop and phone setup specs use the save bar (Reset and Save
  changes) and the phone spec checks the bar is in the viewport without
  horizontal overflow; `mobile-provider-routing-profiles` now saves through
  the bar. All three pass.
