---
id: "02-explain-count-and-cover-surfaces"
title: "Explain the count and verify desktop/mobile"
status: done
wave: 2
depends_on:
  - "01-exclude-copilot-samples"
plan: "plan.md"
requirements:
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
acceptance_criteria:
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.1
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.6
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.7
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.9
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.13
system_design:
  - ../../specs/ui/system-design/context-compaction-count.md
---

# Task 02: Explain the count and verify desktop/mobile

## Summary

Explain counting exclusions through the existing localized info control and public session guide.
Prove zero and retained non-zero counts through focused component and desktop/mobile browser checks.

## In scope

- Update `common:contextCompactionCountHelp` in English, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, and `ko`.
- Generate Traditional Chinese with `pnpm run i18n:zh-hant`; regenerate pseudo with `pnpm run i18n:pseudo`.
- Retain render-time translation, accessible labels, and control behavior.
- Update `TokenUsageDisplay` tests for excluded-stream wording and zero/non-zero counts.
- Extend the desktop and phone help scenarios with a zero fixture and the actual revised explanation.
- Preserve non-zero history coverage. Parameterize `expectCompactionCount` only if needed for zero assertions.
- Retain phone tap toggling and document containment assertions.
- Update the context-window paragraph of `docs/public/sessions-and-review.md` as an explanation section.
- Document missed genuine Copilot compactions, approximate eligible counts, and preserved historical totals.

## Out of scope

- Provider checks in React, new store fields, browser counter arithmetic, or new overlays.
- Mobile layout modernization or unrelated control sizing.
- Public documentation publication before implementation.

## Acceptance

1. Revised help remains accessible through desktop hover/focus and phone touch, with every supported catalog valid.
2. Rendered checks show zero for the correction scenario and preserve a pre-existing non-zero lifetime total.
3. Public documentation explains exclusion and its limits without claiming that ACP cannot report explicit compaction events.

## UI preview

**UI-01: Context-window count and expanded accuracy help**, shared desktop/phone composition.
See the [complete package preview](plan.md#ui-preview).

| Region | Proposed result |
| --- | --- |
| Current usage | Latest observed tokens and source |
| Following row | Compactions: 0, then the existing info control |
| Expanded explanation | Inferred usage drops, the Copilot ACP exclusion, and approximate accuracy |
| Phone interaction | Tap ring, tap info, tap again to close help; the pinned surface remains usable |

The hierarchy and interaction are structural requirements. Copy and spacing are illustrative.
This is a content/data correction within the existing surface, so it needs no separate phone composition.
The closest phone exemplar and rendered check is `mobile-context-window-source.spec.ts`.
Criteria: `.1`, `.6`, `.7`, `.9`, and `.13` under the requirement in frontmatter.

## Verification

Run from the repository root. Install dependencies once for a fresh worktree before any package command.
Managed browser runs rebuild production assets and run sequentially with one shard.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run components/task/chat/token-usage-display.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/context-window-source.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run focused component assertions for RED before changing locale values.
Record discovered browser test counts and actual results.
Browser fixtures prove rendering. Task 01's real backend pipeline test proves the counting correction.
After both work orders pass, synchronize spec lifecycle and plan status as described in the package.

## Files likely touched

- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pt-pt/common.json`
- `apps/web/src/locales/zh-cn/common.json`
- `apps/web/src/locales/zh-hk/common.json` (generated)
- `apps/web/src/locales/zh-tw/common.json` (generated)
- `apps/web/src/locales/ja/common.json`
- `apps/web/src/locales/ko/common.json`
- `apps/web/src/locales/pseudo/common.json` (generated)
- `apps/web/components/task/chat/token-usage-display.test.tsx`
- `apps/web/e2e/tests/chat/context-window-source-helpers.ts`
- `apps/web/e2e/tests/chat/context-window-source.spec.ts`
- `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`
- `docs/public/sessions-and-review.md`

Existing component markup and session-runtime types need no change for this content correction.

## Dependencies

Task 01: backend exclusion and regression coverage.

## Risks

- The longer explanation must remain readable and contained on the phone surface.
- Hardcoded fixture text must not substitute for the actual translated component explanation.
- Store injection cannot prove backend inference; retain the separate real pipeline regression.
- Generated Traditional Chinese or pseudo catalogs can include unrelated changes; inspect the diff.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/context-compaction-count.md), criteria in frontmatter.
- [System design](../../specs/ui/system-design/context-compaction-count.md), presentation and verification sections.
- [Package UI-01](plan.md#ui-preview).
- Existing `ContextCompactionCount`, translated help, and the two context-window source browser suites.
- `apps/web/AGENTS.md`, `docs/i18n.md`, `docs/public/README.md`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.

## Results

Implemented. English, `pt-pt`, `zh-cn`, `ja`, and `ko` explain that Copilot ACP usage is
excluded and eligible counts remain approximate. Traditional Chinese and pseudo catalogs were
regenerated. The unrelated `automations.json` drift produced by the Traditional Chinese generator
was reverted so this change only carries the compaction help.

- Focused component assertions cover the revised explanation and zero/non-zero counts. GREEN:
  `pnpm exec vitest run components/task/chat/token-usage-display.test.tsx` passed 16 tests.
- `pnpm run i18n:check` passed (seven locales complete, pseudo in sync, no em dash, no non-JSX copy);
  `pnpm run i18n:ratchet` passed.
- `pnpm exec oxlint -c oxlint.config.mjs --deny-warnings` passed for the changed web files, and
  `pnpm run typecheck` passed.
- E2E: `pnpm e2e:run --host --shards 1 --project chromium tests/chat/context-window-source.spec.ts`
  passed 3 tests with a zero-count help fixture and the revised explanation.
- E2E: `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts`
  passed 3 tests with tap toggling, containment, a zero-count help fixture, and the revised
  explanation. Non-zero history remains covered by the default fixture in the sibling scenarios.
- Public documentation: the context-window paragraph now explains the Copilot ACP exclusion,
  missed genuine Copilot compactions, approximate eligible counts, and preserved historical totals.
  `node --test scripts/validate-public-docs.test.mjs` passed 63 tests and
  `node scripts/validate-public-docs.mjs` validated 55 pages.
- No React provider checks, new store fields, overlays, or layout changes were added.
