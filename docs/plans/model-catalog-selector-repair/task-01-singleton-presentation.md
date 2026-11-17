---
id: "01-singleton-presentation"
title: "Collapse ordinary singleton presentation"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.5
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.5
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 01: Collapse ordinary singleton presentation

## Summary

Show ordinary singleton models once in the shared selector.
Keep the catalog lossless and preserve meaningful paths, descriptions, source order, and distinct occurrences.

## In scope

- Add rendered coverage in `model-config-selector-singletons.test.tsx` before changing production code.
- Test a singleton beside a real multi-variant group in the same picker.
- Project singleton chains only when their subtree contains one ordinary choice and no composition.
- Preserve multi-choice headings when search leaves one visible result.
- Keep composition detection first, including a single binding and mixed bound/unbound children.
- Use occurrence keys for command identity while selection callbacks receive exact provider values.
- Keep provider descriptions and useful removed path context visible and accessible together.
- Preserve source order across loose leaves, groups, and fallback choices.
- Update the existing catalog suite's `Standard` singleton expectation.

## Out of scope

- Deleting catalog groups, deduplicating models, or changing executable IDs.
- Devin-specific React logic, source acquisition, or a new navigation/store owner.
- A new phone shell or changes to Apply and Save behavior.

## Acceptance

1. The singleton assertion fails on the baseline and passes after the presentation change, while the neighboring variant group remains grouped.
2. Nested paths, search, descriptions, repeated labels/values, source order, selected state, and one-binding compositions retain their required behavior.
3. Desktop and phone component branches use the same projection, with no owner mutation from presentation changes.

## ASCII UI preview

Use [UI-5053-01](plan.md#ui-5053-01-catalog-current-and-corrected-desktop) and [UI-5053-03](plan.md#ui-5053-03-phone-one-inset-sheet).

```text
Desktop popover / phone sheet body
[ Filter models... ]
Kimi K2.7                         [check]
GLM-5.2
  Standard
  1M context
Fusion                                 >
Repeated name
  meaningful group path / description
```

There is no second `Kimi K2.7` heading.
The phone keeps the existing single sheet, internal scroll owner, safe area, and 44px hit areas.
The wider view keeps normal desktop density.
Map this view to AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.2, 002.5, and 004.5.

## Verification

Run the block from the repository root.
The install condition only applies when workspace dependencies are absent.
Task 04 owns browser geometry and full owning-flow acceptance.

```bash
test -d apps/node_modules || (cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/model-config-selector-singletons.test.tsx components/model-config-selector-catalog.test.tsx components/model-config-selector.test.tsx components/model-config-selector-custom-model.test.tsx lib/model-catalog-selection.test.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/model-catalog-groups.tsx components/model-config-selector-rows.tsx components/model-config-selector-singletons.test.tsx components/model-config-selector-catalog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

The permanent RED suite must include both responsive branches, nested singleton chains, and a multi-choice group filtered to one result.
Include repeated exact values under different paths and distinct provider descriptions.
Assert the occurrence count, accessible context, selection callback value, and source order.
Keep `buildCatalogTree`'s singleton preservation test intact.
Add every additional changed frontend path to the focused lint command.

## Files likely touched

- `apps/web/components/model-catalog-groups.tsx`
- `apps/web/components/model-config-selector-rows.tsx`
- New `apps/web/components/model-config-selector-singletons.test.tsx`
- `apps/web/components/model-config-selector-catalog.test.tsx`
- `apps/web/lib/model-catalog-selection.test.ts`, only for a meaningful lossless-tree regression

## Dependencies

None.

## Risks

- A collapse based on filtered children can erase a meaningful variant heading.
- Reusing model IDs as command identities can merge separate catalog occurrences.
- Combining path text with provider descriptions can hide one of them or repeat the singleton name.

## Parallelism

`sequential`

## Inputs

- [Shared selector design](../../specs/agents/system-design/grouped-model-catalog.md#shared-selector)
- [Requirement 002](../../specs/agents/requirements/dynamic-provider-options.md#req-agents-dynamic-provider-options-002-structured-model-catalogs)
- Existing `ModelRow`, `CatalogCommandGroups`, `ModelConfigSheet`, and catalog component tests
- [Issue #5053](https://github.com/kdlbs/kandev/issues/5053)

## Results

Completed 2026-11-04.

- `components/model-catalog-groups.tsx` now projects the full catalog before rendering: an ordinary subtree with one choice and no composition collapses into that choice's single row, and multi-choice groups plus verified compositions keep their presentation.
  Entries render in one ordered traversal, so loose leaves are no longer hoisted above groups.
- `components/model-config-selector-rows.tsx` accepts the occurrence key as the command identity, keeps the exact provider value for selection and selected state, and renders removed path context as a separate accessible detail beside the provider description.
  The flat provider path is unchanged.
- `components/model-config-selector-singletons.test.tsx` is the permanent regression suite: singleton beside a real variant group, nested singleton chains, source order, repeated values under different paths, distinct descriptions, path search, a filtered multi-choice heading, a one-binding composition, and the phone sheet branch.
- `components/model-config-selector-catalog.test.tsx` replaces the former `Standard` singleton-heading expectation with the single row and its context detail.
- `buildCatalogTree` and the catalog wire shape are unchanged; `lib/model-catalog-selection.test.ts` remains the lossless-tree guard.

Verification:

- `(cd apps/web && pnpm exec vitest run components/model-config-selector-singletons.test.tsx components/model-config-selector-catalog.test.tsx components/model-config-selector.test.tsx components/model-config-selector-custom-model.test.tsx lib/model-catalog-selection.test.ts)`: 5 files, 65 tests passed.
- `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/model-catalog-groups.tsx components/model-config-selector-rows.tsx components/model-config-selector-singletons.test.tsx components/model-config-selector-catalog.test.tsx)`: clean.
- `(cd apps/web && pnpm run typecheck)`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `git diff --check`: passed.

Acceptance 1 and 2 are met by those results.
Acceptance 3 is met at the component level for both responsive branches; Task 04 still owns browser geometry and the full owning-flow proof.

Review fixes (2026-11-04):

- Composition command identity is now allocated from one per-render collision-free scheme instead of `composition:<path key>`. `buildCatalogTree` joins unescaped path IDs, so a root group `a/b` and a nested group `b` under `a` produced the same command value; cmdk then marked both entries selected and Enter opened the first one. Compositions now use the validated unique `group.id` base, rows use their leaf occurrence key, and every allocation carries a kind prefix plus a counted suffix so identities stay unique across entry kinds and repeated bases.
  `model-config-selector-singletons.test.tsx` adds a keyboard regression with both slash-ambiguous compositions: distinct `data-value`, exactly one `aria-selected` entry after ArrowDown, Enter opening the nested configuration, and a pointer click opening the root one.
- The removed-path detail now wraps on phone and coarse-pointer layouts (`max-md:whitespace-normal max-md:break-words` plus the `pointer:coarse` variants) instead of ellipsizing, so two long family paths that share a prefix stay visually distinguishable after their heading disappears. Desktop keeps the single-line `truncate` density. The provider description remains its own line and `aria-describedby` still names both.
  A phone regression renders two repeated `Default` labels under long shared-prefix paths and asserts both full paths, the wrapping classes, and the separate descriptions.

- Review follow-up: the catalog `Command` root scores only the declared keywords, so the opaque `row:`/`composition:` identity and its counted suffix never match unrelated models; a regression types `row` and `composition` and expects no results.
- Review follow-up: a top-level loose choice that follows a group keeps its `Model` heading, because the heading now attaches to the first top-level row run regardless of preceding groups; a regression renders a group followed by a loose choice and finds that row inside the named heading group.
- Review follow-up: the acpcompat normalizer validates injected native evidence before indexing it and leaves repeated advertised values unbound so no duplicate row is hidden; both have focused Go regressions.
- Review follow-up: a group whose children are all nested groups force-mounts its cmdk wrapper, so searching a nested choice keeps the parent heading and the matching nested group visible; a regression searches a nested choice and asserts both headings.

Those fixes change no catalog data, no selection callback value, and no Task 02 qualification status.
