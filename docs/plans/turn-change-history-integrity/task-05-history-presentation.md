---
id: "05-history-presentation"
title: "Preserve PR styling and integrate corrected turn data"
status: done
wave: 5
depends_on:
  - 02-history-comparison
  - 03-writer-attribution
  - 04-resumable-export
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.10
  - AC-TASKS-TURN-CHANGES-002.11
  - AC-TASKS-TURN-CHANGES-002.12
  - AC-TASKS-TURN-CHANGES-002.13
  - AC-TASKS-TURN-CHANGES-003.7
  - AC-TASKS-TURN-CHANGES-003.8
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.13
  - AC-TASKS-TURN-CHANGES-004.14
  - AC-TASKS-TURN-CHANGES-005.1
  - AC-TASKS-TURN-CHANGES-005.5
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-005.10
  - AC-TASKS-TURN-CHANGES-005.11
  - AC-TASKS-TURN-CHANGES-005.12
  - AC-TASKS-TURN-CHANGES-005.13
  - AC-TASKS-TURN-CHANGES-006.1
  - AC-TASKS-TURN-CHANGES-006.3
  - AC-TASKS-TURN-CHANGES-006.4
  - AC-TASKS-TURN-CHANGES-006.5
  - AC-TASKS-TURN-CHANGES-006.6
  - AC-TASKS-TURN-CHANGES-006.7
  - AC-TASKS-TURN-CHANGES-007.6
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
  - ../../specs/tasks/system-design/turn-changed-files-subscription.md
---

# Task 05: Preserve PR styling and integrate corrected turn data

## Summary

Integrate corrected turn data into PR #4831's existing presentation.
Keep its normal card, controls, icons, colors, spacing, and mobile composition.
Use existing status/loading areas for exceptions and validate the full user flow.

## Required presentation base

Start on main containing [PR #4831](https://github.com/kdlbs/kandev/pull/4831).
It merged as `f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277` on 2026-10-27.
The merged card, style criteria, and presentation plan were re-read.
Refresh main and verify this ancestry before implementation and final integration.

Read its presentation plan and work order from the resolved base.
Preserve their changes and validation rather than reconstructing styles from the
older card in this draft checkout.
This work order does not authorize reverting or redesigning that PR.

AC-005.10 through .12 retain PR #4831's style meanings.
The earlier conflicting draft status criterion is withdrawn; the minimal UI
integration uses AC-005.13. Keep that reconciliation when replaying the package.

## In scope

- Extend shared types/projection for correct counts, comparison availability, and
  content readiness without exposing internal state as routine product copy.
- Preserve FileIcon, LineStat, status palette, wrapping, disclosure, and the 16px
  message gap from PR #4831 on both normal and fallback transcript anchors.
- Keep successful cards visually unchanged apart from corrected data.
- Label legacy counts as repository changes in the existing count position.
  Do not add another explanation, badge, or per-root basis indicator.
- Reuse one existing unavailable status for an uncomparable turn with no file rows.
  Reuse current Partial/repository status slots for mixed results.
- Keep saved files usable. A selected pending file uses the existing diff loader;
  an affected unavailable file uses its existing error/empty area for the reason.
- Preserve exact selection, subscription recovery, revision merging, current live
  diff modes, scroll ownership, phone drawer geometry, and focus return.
- Reuse existing localized statuses. Add only necessary count wording or selected
  diff failure reasons, with required catalogs and Traditional Chinese conversion.
- Add deterministic isolated mock-agent history scenarios and desktop/phone E2E.
- Update docs/public/tasks-and-workflows.md with the implemented behavior.

## Out of scope

Routine adjustment labels, export-progress messages, extra explanatory rows,
new badges or controls, alternate card styling, new overlays, raw/adjusted toggles,
live DB repair, production seeding endpoints, and unrelated executor qualification.

## Acceptance

1. Normal successful cards retain PR #4831's presentation and interaction structure.
   Files, counts, and patches use one accepted comparison after reload/reconnect.
2. Legacy wording, unavailable/partial states, and selected-file loading reuse
   existing positions. No routine adjustment/saving copy or duplicate unavailable
   message is added. False overlap warnings disappear through corrected data.
3. Desktop/phone history flows and PR #4831's style regressions pass together,
   including icons, colors, spacing, long metadata, focus, touch targets, and overflow.

## ASCII UI preview

Relevant views are UI-01 through UI-03 in the
[combined preview](plan.md#ascii-ui-preview).
Counts and ASCII glyphs are illustrative; PR #4831 defines the actual styles.

```text
Desktop after data correction, same PR #4831 card:
[1 changed file +6 -2] [Expand all] [Open diff]
  v kandev
    v docs/
      [MD icon] plan.md       Modified +6 -2
[card border]
        existing 16px gap
[next message]

Phone, same existing stacked header:
[1 changed file +6 -2]
[Expand all] [Open diff]
  v kandev
    v docs/
      [MD] plan.md   Modified +6 -2
[card border]
        existing 16px gap
[next message]

Legacy, count slot only:
[320 repository changes +13633 -1191] [Open diff]

Unavailable, existing status slot only:
[This turn's change summary is unavailable.]
```

No extra status row appears on success or during background preservation.
The phone card uses transcript scrolling. File taps still open the existing
full-height diff drawer with its own scroll owner and safe-area clearance.
Map: AC-005.7, .10-.13, AC-006.1, .3, .5-.7.

## TDD and regression

Extend projection.test.ts and the card tests inherited from PR #4831.
RED: a history-unavailable result with zero file rows must use the existing
unavailable status; the older visibility predicate hides it.
Also assert legacy count wording, available totals for mixed roots, pending-file
loading, revision-safe updates, and exactly one unavailable message.

For direct and adjusted success, assert the same controls and information rows
as PR #4831. Assert no routine adjustment badge, saving message, per-root basis
label, or extra explanatory paragraph. Preserve its filename/fallback icons,
LineStat/status colors, outer spacing, and path disclosure tests.

Add proposed desktop tests/git/turn-changes-history-integrity.spec.ts and
mobile tests/git/mobile-turn-changes-history-integrity.spec.ts.
Follow the plan's E2E matrix while retaining the PR's existing desktop/mobile
changed-files and scroll suites. Cover 390px fine/coarse pointers, 767px and
768px wrapping, long metadata, and normal/fallback transcript anchors.

Use real Git operations inside the admitted mock turn and fixture barriers
instead of sleeps. Restore preferences and remove owned test resources.
Legacy/failure records belong only in isolated E2E storage.
Do not use the developer instance or add production seed endpoints.

## Verification


Run dependency installation once if the worktree lacks apps/node_modules.
All commands below are independently rooted at the repository.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/turn-changes/projection.test.ts components/task/chat/turn-changed-files-card.test.tsx hooks/domains/session/use-turn-changes-subscription.test.tsx hooks/domains/session/use-turn-changes-pagination.test.tsx lib/state/slices/session/turn-changes-actions.test.ts lib/ws/handlers/turn-changes.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/types/turn-changes.ts lib/turn-changes/projection.ts components/task/chat/turn-changed-files-card.tsx components/task/historical-turn-diff-viewer.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/git/turn-changes-history-integrity.spec.ts tests/git/turn-changed-files.spec.ts tests/git/turn-changed-files-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/git/mobile-turn-changes-history-integrity.spec.ts tests/git/mobile-turn-changed-files.spec.ts tests/git/mobile-turn-changed-files-scroll.spec.ts)
(cd apps/backend && GOMAXPROCS=2 go test -p 2 -trimpath -tags fts5 ./cmd/mock-agent -run 'TestTurnHistoryIntegrityScenario' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Desktop and phone commands run sequentially and rebuild the production app.
Confirm each proposed spec is discovered; no-tests-found is not a pass.
Record exact test counts, rendered comparison with UI-01 through UI-03, locale
checks, and any unavailable PostgreSQL/executor evidence.
Run changed-Go lint for the mock fixture files. Add the proposed
cmd/mock-agent/turn_history_integrity_test.go with TestTurnHistoryIntegrityScenario.

## Files likely touched

Existing after the PR #4831 base: apps/web/lib/types/turn-changes.ts;
apps/web/lib/turn-changes/projection.ts;
apps/web/components/task/chat/turn-changed-files-card.tsx;
apps/web/components/task/chat/turn-changed-files-card.test.tsx;
apps/web/components/task/historical-turn-diff-viewer.tsx;
apps/web/hooks/domains/session/use-turn-changes.ts;
apps/web/lib/state/slices/session/turn-changes-actions.ts;
apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json;
apps/backend/cmd/mock-agent/;
apps/web/e2e/tests/git/turn-changed-files-helpers.ts;
docs/public/tasks-and-workflows.md.

The new history-integrity specs, mock scenario test, and focused helpers are
proposed additions. Preserve the PR's card classes and shared primitives.
The mobile wrapper should need no styling changes; shared data/projection and
the existing drawer remain the integration points.

## Dependencies

02-history-comparison, 03-writer-attribution, 04-resumable-export, and the resolved
PR #4831 presentation base. The external PR is recorded in the plan because
work-order depends_on contains only sibling work-order IDs.

## Risks

A zero-entry unavailable state is a deliberate visibility exception.
A stale response must not restore broad totals or overwrite newer content state.
Preserve PR #4831's wrapping, icons, colors, touch geometry, and message spacing
when integrating changes in the same card component and specifications.
Legacy precision must not be invented from today's checkout.

## Parallelism

sequential

## Inputs

- History integrity design: Required presentation baseline; Projection and presentation; Historical navigation and mobile.
- Plan UI-01 through UI-03 and its E2E matrix.
- PR #4831 presentation plan/work order, card source, tests, and both screenshots.
- Existing changed-file/scroll specs and mock-agent scoped instructions.
- Subscription reconciliation tests; mobile-parity, e2e, and docs-maintainer guidance.

## Results

Implemented shared comparison/export projection, repository count basis, counts availability, and legacy count wording. The card reuses PR #4831 styling and controls, shows one unavailable status for an uncomparable turn, keeps pending content quiet, and labels legacy counts in the existing count slot. Added localized count and selected-diff failure copy, synthetic-selection readiness handling, and production viewer coverage for late-page synthetic selections.

Added the deterministic mock-agent `turn-history-integrity` and `turn-history-conflict` scenarios plus a conflict setup scenario. Added desktop and phone history-integrity specs and browser coverage for conflict/unavailable, legacy repository wording, mixed available/unavailable roots, real versus stale overlap notes, and paused content with saved-file readiness. The phone spec also covers dark mode and a pseudo-locale card/drawer with no horizontal overflow. Existing PR #4831 desktop and phone style suites and both scroll suites pass unchanged.

Executed in this environment:

- Six desktop history specs together in one worker: 6 passed.
- Phone history spec: 2 passed (main drawer flow with dark mode, and pseudo-locale containment).
- PR #4831 desktop style: 6 passed; phone style: 3 passed; desktop scroll: 1 passed; mobile scroll: 1 passed.
- Frontend focused Vitest: 83 passed across 15 files; TypeScript typecheck passed.
- Mock-agent scenario tests, turn-change, SQLite, agentctl process, lifecycle, and export integration Go tests passed; store conformance and `cmd/sqlguard` passed; changed-Go lint reported zero issues.
- i18n keys/ratchet and public-doc/specification validation passed.

The history specs reset the shared worker checkout before seeding so their untracked-file turns remain independent. Not executed: PostgreSQL-backed browser evidence (no `KANDEV_TEST_POSTGRES_DSN`). Mixed roots use an isolated retained-record fixture because the browser fixture has no two-healthy-repository capture.

### Fixup remediation (2026-10-28)

A synthetic selection now retries once when its repository finishes, so a saved late-page patch is not left in the unavailable state. The hook and mounted viewer tests cover the repository-settled transition.
