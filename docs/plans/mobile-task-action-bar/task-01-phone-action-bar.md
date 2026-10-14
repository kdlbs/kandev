---
id: "01-phone-action-bar"
title: "Repair and prove the phone action bar"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-ACTION-BAR-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.1
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.2
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.3
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.4
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.5
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.6
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.7
  - AC-UI-MOBILE-TASK-ACTION-BAR-001.8
system_design:
  - ../../specs/ui/system-design/mobile-task-action-bar.md
---

# Task 01: Repair and prove the phone action bar

## Summary

This completed work order records the original action order. The later
normal-chat phone placement is owned by
[the composer follow-up](../mobile-composer-action-wrapping/task-02-place-action-above-utilities.md).
Its UI-02 supersedes this work order's workflow-versus-utility order only;
notice placement, target sizing, and recorded results remain this work order's
delivery evidence.

Repair the phone tray and shared terminal notice as one vertical slice. Preserve
domain callbacks, prove touch geometry and real archive/workflow outcomes, and
update localized copy and the small public phone-controls explanation.

## In scope

- Phone notice/action separation in both chat and passthrough, shared terminal
  notice layout/copy, complete-control wrapping, affected control target sizing.
- Focused component regressions and phone/desktop Playwright scenarios in the
  plan; translated strings and docs paragraph.

## Out of scope

- New domain state, persistence, backend, dialogs, navigation, or global CSS audit.
- PR publication and merge, owned by Task 02.

## Acceptance

- Match UI-01/UI-02/UI-03 at phone, boundary, coarse tablet, and desktop widths
  with readable status, contained complete controls, and measured target sizes.
- Retain archive preference/cancel/confirm, per-task dismissal persistence,
  utility actions, and workflow ownership; cover alternate/pending states.
- Localize changed copy, pass exact checks, and record actual results before
  marking done. Promote paired specs only when all criteria are satisfied.

## ASCII UI preview

UI-01: Phone task composer, merged PR. See [full previews](plan.md#ascii-ui-preview).

```text
+---------------------------------------+
| merge PR #3948 merged   [Archive] [x] |
+---------------------------------------+
| [Todos] [nav] [Usage] [Share] [Done >] |
| extra complete controls wrap here    |
+---------------------------------------+
| Message composer ...                 |
+---------------------------------------+
```

UI-02: Desktop task composer, merged PR.

```text
| Todos | PR #3948 has been merged. ... Archive x | nav Usage Share Done > |
```

UI-03: Phone alternate/pending states: closed-without-merge message wraps in
the notice region; absent notice removes the row; busy Proceed retains geometry
and disabled behavior; Archive uses the existing confirmation and Cancel restores
the task/notice. Structure is required, spacing is illustrative. No new scroll
owner; 44px targets for phones/coarse pointers. Covers AC-001.1 through 001.8
of this work order's requirement.

## Verification

Run from the repository root. Install once in a fresh worktree before package
commands. Browser runs rebuild current production assets; do not overlap them.

```bash
export PATH="$HOME/.local/share/mise/shims:$PATH"
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/chat-status-bar.test.tsx components/task/chat/pr-archive-banners.test.tsx components/task/passthrough-toolbar.test.tsx components/task/chat/todo-indicator.test.tsx components/task/chat/auto-scroll-toggle-button.test.tsx components/task/chat/conversation-usage-display.test.tsx components/task/chat/jump-to-latest-button.test.tsx components/task/chat/scroll-to-last-prompt-button.test.tsx components/task/workflow-move-proceed-button.test.tsx components/threads/open-in-threads-button.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/chat-status-bar.tsx components/task/chat/pr-archive-banners.tsx components/task/chat/transcript-nav-group.tsx components/task/chat/todo-indicator.tsx components/task/chat/auto-scroll-toggle-button.tsx components/task/chat/conversation-usage-display.tsx components/task/chat/jump-to-latest-button.tsx components/task/chat/scroll-to-last-prompt-button.tsx components/task/share/share-button.tsx components/task/workflow-move-proceed-button.tsx components/task/passthrough-toolbar.tsx components/threads/open-in-threads-button.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-chat-status-bar.spec.ts tests/cli-mode/mobile-passthrough-composer.spec.ts tests/workflow/mobile-workflow-step-proceed.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/chat-status-bar.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

If adding/changing another component suite, append its exact command and run it
before done; specifically add `components/task/share/share-button.test.tsx` if
ShareButton gains logic. No generic second verification pass is required.

## Files likely touched

- `apps/web/components/task/chat/chat-status-bar.tsx` and its test.
- `apps/web/components/task/chat/pr-archive-banners.tsx` and its test.
- `apps/web/components/task/chat/transcript-nav-group.tsx`.
- Affected standalone controls under `apps/web/components/task/chat/`: todo,
  auto-scroll, usage, jump-to-latest, scroll-to-last-prompt, and their tests.
- `apps/web/components/task/share/share-button.tsx` and a focused test if needed.
- `apps/web/components/task/workflow-move-proceed-button.tsx` and test if needed.
- `apps/web/components/task/passthrough-toolbar.tsx` and its test.
- `apps/web/components/threads/open-in-threads-button.tsx` only if its existing
  target needs sizing; add its exact affected test command if changed.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- `apps/web/e2e/tests/chat/mobile-chat-status-bar.spec.ts` (new).
- `apps/web/e2e/tests/chat/conversation-usage-helpers.ts`.
- `apps/web/e2e/tests/chat/chat-status-bar.spec.ts`.
- `apps/web/e2e/tests/cli-mode/mobile-passthrough-composer.spec.ts`.
- `apps/web/e2e/tests/workflow/mobile-workflow-step-proceed.spec.ts`.
- `docs/public/mobile-remote-access.md`.
- Paired specs and this plan/work-order status/results.

## Dependencies

None. Implementation requires the later explicit implementation request.

## Risks

See [plan risks](plan.md#risks). Inspect actual containing rows when increasing
targets; preserve pointer-based disclosure independently of phone sizing.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-task-action-bar.md).
- [Design](../../specs/ui/system-design/mobile-task-action-bar.md).
- Existing archive banner/component tests and chat status-bar desktop E2E.
- Existing mobile archive preference and workflow proceed E2E patterns.
- `/tdd`, `/e2e`, `/mobile-parity`, `/docs-maintainer`, and `apps/web/AGENTS.md`.

## Results

Completed on 2026-10-12 in the primary conversation. No delegated agents.

- Fresh workspace `pnpm install --frozen-lockfile`: passed (935 packages).
- The exact Vitest block above, including the affected Threads button suite:
  10 suites, 116 tests passed. New concise messages failed first on the old copy.
- Typecheck and the exact scoped ESLint command above: passed.
- `i18n:zh-hant` and `i18n:pseudo`: generated the affected catalogs. Unrelated
  pre-existing generator drift was excluded from this change.
- `i18n:check` and `i18n:ratchet`: passed; all seven real locales plus pseudo
  cover the changed notice and auto-scroll labels.
- Managed mobile command above: 10 scenarios passed, including passthrough
  and task-owned workflow movement. Log: `/tmp/kandev-run.e2e.EBNl44jX.log`.
- Managed desktop command above: 7 scenarios passed.
  Log: `/tmp/kandev-run.e2e.eMZ7rgwD.log`.
- Final capture and fixture-cleanup refinement reran all four new regressions:
  `CAPTURE_PR_ASSETS=1 scripts/run-quiet e2e --summary -- pnpm --dir apps/web
  e2e:run --no-build --project mobile-chrome tests/chat/mobile-chat-status-bar.spec.ts`:
  4 passed. Production assets were unchanged from the preceding managed build.
  Log: `/tmp/kandev-run.e2e.rxInsBWc.log`.
- Red browser evidence: Archive was 16px high before the repair, versus the
  required 44px. Green geometry covers 320/393/640/767px, 768px fine pointer,
  desktop fine pointer, coarse tablet, and a long workflow label.
- Catalog validation, all spec lint, and 36 spec-linter tests: passed.
- Public docs: 63 validator tests and validation of 47 published pages passed.
- Documentation coverage preflight: both work orders covered, no errors.
- `git diff --check`: passed. Rendered results match UI-01/UI-02/UI-03; existing
  confirmation and usage/share surfaces remain the feature owners.

### PR remediation validation

Frontend CI found that the new eligibility test's array rows were unpacked as
individual arguments. Named object rows now pass the complete PR arrays to the
real eligibility branches. The final `pnpm run typecheck` passes.

A valid review finding identified nowrap text on coarse tablets. The stronger
800px regression failed with `white-space: nowrap` before the change. Coarse
pointer labels now wrap; the browser checks both the computed style and text
containment inside the button. The desktop regression also measures zero
left auto-margin on Proceed at 768px and 1280px.

After those changes, the two affected component suites passed all 21 tests,
typecheck and zero-warning scoped ESLint passed, and the four action-bar E2E
scenarios passed with a fresh production build. Log:
`/tmp/kandev-run.e2e.r6APtnGY.log`. A post-commit capture recheck is recorded
externally with the final media/head evidence.
