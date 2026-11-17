---
id: "01-update-generic-help"
title: "Make generic compaction help provider-neutral"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
acceptance_criteria:
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.1
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.6
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.7
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.14
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.15
system_design:
  - ../../specs/ui/system-design/context-compaction-count.md
---

# Task 01: Make Generic Compaction Help Provider-Neutral

## Summary

Remove the Copilot exclusion sentence from the shared inferred-count explanation in every supported locale.
Use the existing component and browser cases to prove the shorter explanation and retained count behavior.

## In scope

- Revise the existing inferred-count component assertions before changing the locale values.
- Cover missing reporting metadata and explicit `usage_drop` reporting in the inferred-count case.
- Require inference and accuracy explanations without Copilot text for zero and non-zero totals.
- Preserve the existing native, partial, and unavailable coverage cases.
- Update `contextCompactionCountHelp` in `en`, `pt-pt`, `zh-cn`, `ja`, and `ko`.
- Generate `zh-tw` and `zh-hk` through `i18n:zh-hant`, then regenerate `pseudo` through `i18n:pseudo`.
- Update the two existing desktop/phone help scenarios to assert the real translated explanation.
- Preserve hover/focus access, phone second-tap dismissal, count assertions, and the existing overflow assertion.
- Record the exact RED result, final check results, and rendered UI comparison in this work order and its plan.

## Out of scope

- Production changes to `TokenUsageDisplay`, `compactionHelpKey`, stores, or parsers.
- New locale keys, provider checks, agent lookups, schema changes, or runtime flags.
- Changes to native Copilot messages or provider counting policies.
- Mobile layout modernization, unrelated control sizing, or public documentation changes.

## Acceptance

1. Generic help in every shipped locale explains inference and accuracy without the Copilot sentence. Counts and accessible controls remain present.
2. Existing native coverage cases retain their explanations and statuses. No production provider dispatch changes are required.
3. The revised component assertion first fails on the current copy. Final component, localization, desktop, and phone checks pass.

## UI preview

**UI-01: Expanded inferred-count help**, shared desktop/phone composition.
See the [complete package preview](plan.md#ui-preview).

The existing row shows the Compactions label, retained count, and info control.
The expanded English explanation is:

> Kandev infers this count from context usage drops. Missing samples or provider resets can affect accuracy.

On phones, the current ring/info tap path and second-tap dismissal remain in place.
This shorter copy needs no new surface, scroll region, or phone composition.
The closest shipped exemplar and rendered check is `mobile-context-window-source.spec.ts`.
Content and reachability are required. Wrapping and spacing are illustrative.
Criteria: `.1`, `.6`, `.7`, `.14`, and `.15` under the requirement in frontmatter.

## Verification

Run from the repository root.
For a fresh worktree, install workspace dependencies once before the first package command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

First revise the component expectation, then run its focused suite to capture a failure on the existing Copilot text.
After the locale edits, use these generation commands:

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
```

Inspect the generated diff before the final checks.
Run the browser commands sequentially. Each managed command rebuilds the production assets and uses one shard.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/token-usage-display.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/context-window-source.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/ui docs/plans/generic-compaction-help apps/web
git status --short -- docs/specs/ui docs/plans/generic-compaction-help apps/web
```

The full component suite retains keyboard, Escape, zero/non-zero, and native coverage assertions.
The two browser files provide the focused rendered desktop and phone evidence for UI-01.
Confirm test discovery and record actual results rather than copying historical test counts.
No backend suite or broad frontend verification is required for this locale-only production change.

## Files likely touched

- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pt-pt/common.json`
- `apps/web/src/locales/zh-cn/common.json`
- `apps/web/src/locales/ja/common.json`
- `apps/web/src/locales/ko/common.json`
- `apps/web/src/locales/zh-tw/common.json` (generated)
- `apps/web/src/locales/zh-hk/common.json` (generated)
- `apps/web/src/locales/pseudo/common.json` (generated)
- `apps/web/components/task/chat/token-usage-display.test.tsx`
- `apps/web/e2e/tests/chat/context-window-source.spec.ts`
- `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`
- `docs/plans/generic-compaction-help/plan.md` and this work order, for status and results.

## Dependencies

None. The existing help dispatch and shipped source-help fixtures already support this correction.
The separate native projection parity blocker does not block this work order.

## Risks

- Generated catalogs can contain unrelated changes. Review each changed key and preserve other workspace edits.
- The negative provider assertion must target generic help, because native Copilot help still names Copilot.
- Existing historical totals and native coverage caveats must remain intact.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/context-compaction-count.md), criteria in frontmatter.
- [System design](../../specs/ui/system-design/context-compaction-count.md#generic-help-and-provider-details).
- [Plan](plan.md), including the compatibility matrix and UI-01.
- `apps/web/components/task/chat/token-usage-display.tsx`, especially `compactionHelpKey` and `ContextCompactionCount`.
- Existing component cases and `apps/web/e2e/tests/chat/context-window-source-helpers.ts`.
- `apps/web/AGENTS.md`, `apps/web/components/task/chat/AGENTS.md`, `docs/i18n.md`, `/tdd`, `/e2e`, and `/mobile-parity`.

## Results

Done. The inferred-count help now explains usage-drop inference and accuracy limits in all seven shipped locales.
The Copilot exclusion sentence remains only in the counting policy and the native Copilot messages.
`compactionHelpKey` and `ContextCompactionCount` needed no production change.

RED: after the component assertions required provider-neutral help and rejected `Copilot` in the selected explanation, the focused suite failed all three generic-help cases (`missing reporting metadata`, `explicit usage drop reporting`, and the zero-count case) with the existing English value.
The other 17 cases stayed green.

Final checks:

- `cd apps/web && pnpm exec vitest run components/task/chat/token-usage-display.test.tsx` — 20 passed.
- `cd apps/web && pnpm run i18n:check` — passed; all locales complete and pseudo in sync.
- `cd apps/web && pnpm run i18n:ratchet` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Focused Oxlint over `token-usage-display.test.tsx` and the two browser specs — passed.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/context-window-source.spec.ts` — 3 passed.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts` — 3 passed.
- `python3 scripts/list-docs.py validate` — passed (436 decisions, 1,931 specifications).
- `python3 scripts/lint-spec-files.test.py` — 36 passed.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `git diff --check -- docs/specs/ui docs/plans/generic-compaction-help apps/web` — passed.

Rendered UI comparison: UI-01 keeps the Compactions label, the count, the info control, and the same help region.
The expanded explanation is now two sentences without Copilot text on desktop hover and phone tap.
The native, partial, and unavailable cases still show their provider-specific explanations and status badges.
The phone second-tap dismissal, overflow assertion, and desktop keyboard/Escape behavior remain green.
`apps/web/src/locales/{en,pt-pt,zh-cn,ja,ko}/common.json` changed only `contextCompactionCountHelp`.
`i18n:zh-hant` and `i18n:pseudo` regenerated the corresponding key in `zh-tw`, `zh-hk`, and `pseudo` with no unrelated catalog changes.
