---
id: "01-query-recovery"
title: "Saved-query recovery"
status: complete
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.1
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
---
# Saved-query recovery

## Summary and scope

Implement the query recovery section of the [design](../../specs/integrations/system-design/github-dashboard-mobile.md), including its focused regressions and translations. Own only the files listed below and required adjacent tests.

## Out of scope

Other work-order outcomes and changes to persistence or provider API contracts.

## Acceptance

- The referenced acceptance criteria pass through the real interaction/state boundary.
- Failure paths retain prior state and permit recovery; desktop behavior remains intact.
- Required targeted checks pass and results are recorded.

## ASCII UI preview

### UI-01: Saved-query recovery

```text
Loading views...
Unable to load views  [Retry]
Save query: [Name] [Repository]
[Cancel] [Save]  (pending prevents duplicates)
```

See the [combined preview](plan.md#ascii-ui-preview). Geometry and hierarchy are structural; ASCII spacing is illustrative.

## Verification

From the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/github/my-github/use-saved-presets*.test.ts components/github/my-github/use-saved-preset-actions.test.ts)
```

Hook regressions use Red-Green-Refactor. Run new/changed tests after the final source edit.

## Files likely touched

Paths relative to apps/web: `components/github/my-github/use-saved-presets.ts`, `use-saved-preset-actions.ts`, `save-preset-dialog.tsx`, `saved-query-load-status.tsx`, and the shared saved-menu status slot.

## Dependencies

None.

## Risks

Load failure/retry and save completion must preserve workspace ownership and existing mutation rollback.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/integrations/requirements/github-dashboard-mobile.md), paired design, existing source and tests named above.

## Results

Verified deferred load/save failures, successful empty collections, retry, duplicate-submit prevention, optimistic rollback, mutation ordering, and stale workspace responses. The focused hook/form tests were observed failing before implementation, then passing. Mobile persistence/recovery tests and desktop saved-query regressions pass. Desktop Retry uses the menu's keyboard navigation semantics.
