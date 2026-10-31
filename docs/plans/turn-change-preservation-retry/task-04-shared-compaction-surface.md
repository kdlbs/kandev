---
id: "04-shared-compaction-surface"
title: "Shared compaction surface and end-to-end evidence"
status: completed
wave: 4
depends_on:
  - "03-unified-history-compaction"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.8
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.10
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.12
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.5
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-006.1
  - AC-TASKS-TURN-CHANGES-006.5
system_design:
  - ../../specs/system-page/system-design/history-compaction.md
  - ../../specs/tasks/system-design/turn-change-preservation.md
---

# Task 04: Shared compaction surface and end-to-end evidence

## Summary

Present the broader cleanup scope through the existing single card and action set. Prove Git-backed historical reads and unified compaction on desktop and phone.

## In scope

Localized History compaction copy, one control set, category result details, stale-scope analysis, preparation/backup feedback, historical ready/expired projection, public docs, and focused E2E.

## Out of scope

Extra history controls, a new settings page, per-category schedules, and unrelated database cleanup.

## Acceptance

1. Desktop and phone expose one period, analysis action, switch, manual action, and preparation choice for both categories.
2. One mixed-data cleanup preserves prose and turn/file summaries while tool details show removed state and historical content shows expired state.
3. On-demand Git rendering, reload, reconnect, exact historical selection, safe-area layout, and phone focus return remain correct.

## ASCII UI preview

UI-01 and UI-02 from the [combined plan](plan.md#ascii-ui-preview) define the affected states.

```text
History compaction
Tasks inactive for [3] [Months]   [Analyze savings]
Analysis details: tool details + historical diffs
Automatic compaction [toggle]
[Compact history now]
```

Desktop and phone keep the existing stacked composition and action order. Phone retains 44px controls and one page scroll owner.

## Verification

Record a behavioral RED regression before production changes. Use real Git/database fixtures for source and cleanup claims.

```bash
pnpm --dir apps/web test components/settings/system/tool-payload-retention-card.test.tsx lib/turn-changes/projection.test.ts
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:pseudo
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/system/tool-payload-retention.spec.ts tests/git/turn-changed-files.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/system/mobile-tool-payload-retention.spec.ts tests/git/mobile-turn-changed-files.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run changed-package Go lint with actual paths. For web commands, install workspace dependencies from `apps/` if this worktree lacks them.
Use the configured PostgreSQL conformance fixtures where applicable, or record missing coverage explicitly.

## Files likely touched

- `apps/web/components/settings/system/tool-payload-retention-card.tsx`
- `apps/web/components/settings/system/tool-payload-retention-fields.tsx`
- `apps/web/components/settings/system/tool-payload-retention-results.tsx`
- `apps/web/hooks/domains/system/use-tool-payload-retention.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/system.json`
- `apps/web/e2e/tests/system/tool-payload-retention.spec.ts`
- `apps/web/e2e/tests/system/mobile-tool-payload-retention.spec.ts`
- `apps/web/e2e/tests/git/turn-changed-files.spec.ts`
- `apps/web/e2e/tests/git/mobile-turn-changed-files.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/git-operations.md`

Add focused adjacent regression files. Extract helpers rather than exceeding scoped size limits.

## Dependencies

03-unified-history-compaction

## Inputs

- [System design](../../specs/system-page/system-design/history-compaction.md)
- [System design](../../specs/tasks/system-design/turn-change-preservation.md)
- Root and scoped AGENTS guidance, relevant existing tests, and the accepted decision.

## Risks

Report SQL reuse and actual Git reclamation accurately. Do not add a second policy while adding category details.

## Parallelism

`sequential`

## Results

The existing Database card now presents History compaction with one period, analysis action, switch, manual action, and preparation choice.
Results separate tool-detail, SQL-history, and Git-checkpoint categories, and expired/retry counts use the same result panel.
A pending preservation projects a preparing repository state instead of a final loss result.
Copy is localized in all eight catalogs, including generated zh-hant and pseudo catalogs.

Behavioral evidence:

- `pnpm test components/settings/system/tool-payload-retention-card.test.tsx lib/turn-changes/projection.test.ts`
- Desktop `tests/system/tool-payload-retention.spec.ts`: 5 passed, including the new mixed-category test.
- Mobile `tests/system/mobile-tool-payload-retention.spec.ts`: 5 passed, including the new mixed-category test.
- Desktop `tests/git/turn-changed-files.spec.ts`: 6 passed.
- Mobile `tests/git/mobile-turn-changed-files.spec.ts`: 3 passed.
- `pnpm run i18n:check`, `pnpm run typecheck`, public-docs validation, and specification lint passed.

Remaining gap: no dedicated browser fixture creates a real Git-backed historical turn and then expires it; expiry projection is covered by unit projection tests and backend expiry tests.
