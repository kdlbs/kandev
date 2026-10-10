---
id: "01-restore-measured-geometry"
title: "Restore measured timeline geometry"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.4
  - AC-UI-BOUNDED-CHANGES-001.6
  - AC-UI-BOUNDED-CHANGES-001.7
  - AC-UI-BOUNDED-CHANGES-001.8
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 01: Restore Measured Timeline Geometry

## Summary

Repair presentation invalidation so unchanged mounted rows regain actual sizes
before anchor restoration. Add regression coverage that proves the failure
without another row observer event and verifies rendered desktop/phone spacing.

## In scope

- Implement the bounded refresh described in the owning design and plan.
- Prove compact and variable row geometry, hidden fallback, stable anchor,
  bounded mount count, and context replacement with TDD.
- Add actual commit-history geometry checks beside existing large Changes tests.

## Out of scope

Shared Files measurement changes, new dependencies, CSS density changes,
Git/provider data changes, full-suite runs, and publication.

## Acceptance

- Font, locale, width, and pointer invalidation leave mounted row transforms
  consistent with actual sizes even when unchanged rows emit no observer entry.
- The visible key/offset survives a same-context refresh; hidden rows retain
  positive keyed geometry, and replacement contexts discard stale measurements.
- Production-served desktop and phone regressions prove contiguous commit rows,
  bounded mounting, and existing usable actions, including the mobile breakpoint.

## ASCII UI preview

UI-01: Changes history after presentation invalidation, excerpt from
[the plan](plan.md#ascii-ui-preview).

```text
Desktop Changes          Phone Changes
Commits                  +------------------------+
o abc1234 first commit   | Changes                |
o def5678 second commit  | o abc1234 first   [Open]|
o 901abcd third commit   | o def5678 second  [Open]|
                         +------------------------+
                         | Chat | Files | Changes |
                         +------------------------+
```

Row wrappers are contiguous according to actual measured heights, not a fixed
shared pixel height. Existing phone Changes is the exemplar: bottom-navigation
entry, fixed header/navigation, one content scroller, and 44px action targets.
The sketch is illustrative; geometry assertions prove .6 and .8.

## Likely files

- `apps/web/components/task/changes-timeline-measurement.ts`
- `apps/web/components/task/changes-timeline-measurement.test.ts`
- `apps/web/components/task/changes-timeline-viewport.tsx`
- `apps/web/components/task/changes-timeline-viewport.test.tsx`
- `apps/web/e2e/tests/git/changes-commit-spacing-helpers.ts`
- `apps/web/e2e/tests/git/large-changes-virtualization.spec.ts`
- `apps/web/e2e/tests/git/mobile-large-changes-virtualization.spec.ts`

## Implementation sequence

1. Mark this work order in progress after the explicit implementation request.
2. Add a controlled-observer regression with real 24px measured rows and a 34px
   estimate. Emit positive row measurements once, trigger font/locale/width
   invalidation, and keep row observers silent. Assert transforms, not just
   `measure()` call counts. Record the expected failure before production edits.
3. Add the mounted-only refresh helper. Snapshot current indices, keys, positive
   heights, and hidden fallback before invalidation. Clear/rebuild through public
   virtualizer APIs, apply sizes, then publish the anchor for restoration.
4. Add hidden, mixed-height, anchor, and replacement-context coverage. Do not
   alter `measureFileTreeElement` or increase overscan.
5. Seed collapsed commits in isolated browser fixtures using the existing
   environment/session mapping. Assert adjacent visible wrapper boundaries and
   actual commit header sizing after font invalidation, scroll, resize, and
   hide/reopen. Assert visible key/offset instead of constant raw scrollTop.
6. Exercise desktop fine pointer, coarse phone at 393px/767px, and the 768px
   desktop boundary. Keep the existing large-collection and source-routing tests.
7. Run every check below, record actual results and RED evidence, and mark done.

## Verification

Run from the repository root. Workspace dependencies were installed during
diagnosis; reinstall with `(cd apps && pnpm install --frozen-lockfile)` if missing.

```bash
(cd apps/web && pnpm exec vitest run components/task/changes-timeline-measurement.test.ts components/task/changes-timeline-viewport.test.tsx components/task/file-tree-measurement.test.ts components/task/changes-timeline-interaction.test.tsx components/task/commit-row.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/changes-timeline-measurement.ts components/task/changes-timeline-measurement.test.ts components/task/changes-timeline-viewport.tsx components/task/changes-timeline-viewport.test.tsx e2e/tests/git/large-changes-helpers.ts e2e/tests/git/changes-commit-spacing-helpers.ts e2e/tests/git/large-changes-virtualization.spec.ts e2e/tests/git/mobile-large-changes-virtualization.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/git/large-changes-virtualization.spec.ts tests/git/commit-file-navigation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-large-changes-virtualization.spec.ts tests/git/mobile-commit-file-navigation.spec.ts tests/task/mobile-changes-panel.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed runs build fresh production assets. Retain each long-running handle and
poll it to terminal completion. Do not overlap suites or override worker limits.
Pointer/locale/font unit triggers must actually fire and return settled geometry.
No fixed sleeps are needed; use existing causal/geometry polling helpers in E2E.

## Dependencies

None. The installed TanStack APIs and current row markers provide the mechanism.

## Risks

Anchor publication order, zero hidden geometry, stale row indices, and geometry
reads outside the invalidation boundary. Browser assertions are required to catch
observer-delivery behavior that happy-dom cannot reproduce.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/bounded-changes-rendering.md), .4/.6/.7/.8.
- [Design](../../specs/ui/system-design/bounded-changes-rendering.md#measurement-and-lifecycle).
- [Confirmed reproduction](plan.md#confirmed-cause-and-reproduction).
- Existing measurement, viewport, commit navigation, and large Changes tests.

## Results

Completed on 2026-09-30. Presentation refresh snapshots only mounted positive
heights (or keyed hidden fallback), resets/rebuilds the virtualizer, restores
sizes, then publishes the saved anchor. Shared Files measurement is unchanged.

RED evidence: after a single real-size observation, font/locale/pointer refresh
without further row entries produced estimate-based transforms
`[0, 34, 68, 102, 136]` instead of `[0, 24, 76, 100, 136]`: three failures.
The width case already recovered via owner-observer delivery. After the repair,
all four cases pass without new row observations.

- Five focused unit files: 43 tests passed, including mixed heights, positive
  hidden fallback, stale/disconnected wrappers, anchor/context compatibility.
  A later test-selector cleanup was verified by all 11 viewport tests.
- Web typecheck and changed-file ESLint (including the new E2E helper) passed.
- Full i18n check and changed-code ratchet passed.
- Fresh production desktop E2E: 5 passed, including both 50,000-file layouts,
  desktop/tablet commit navigation, contiguous spacing, resize, and reopening.
- Fresh production phone E2E: 13 passed, including 393px/767px spacing,
  measured anchor preservation, touch targets, navigation, and Changes sheets.
- Spec catalog validation (336 decisions, 1,269 specifications), full spec lint,
  and whitespace checks passed.
- Synthetic desktop and phone capture manifests were preserved separately
  before managed-runner cleanup. Full project validation and publication are
  tracked in Task 02.

The browser refresh helper was refined to await two animation frames after
font invalidation; the phone focused run used this final test helper, and the
CI in Task 02 validates its final desktop use after the user stopped local tests.
