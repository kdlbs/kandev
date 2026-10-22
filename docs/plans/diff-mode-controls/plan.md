---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-DIFF-PRESENTATION-001
system_design:
  - ../../specs/ui/system-design/diff-presentation-preferences.md
legacy_specs: []
---

# Synchronize Diff Mode Controls

## Overview

One sequential work order reconnects the two topbar state adapters to the existing
global hook. The user explicitly authorized the bounded design package and TDD in
the same turn, with a committed parent-validation stop before push or PR.

## Scope and technical approach

The parent diagnostic proved actual Review file-header unified-to-split changes
leave the mounted topbar stale and its next click repeats split. Both Review and
Changes independently initialize `splitView` once and manually write/broadcast.
Reuse `useGlobalViewMode` for those booleans and setters. Do not modify the hook,
raw preference protocol, defaults, wrap/review policy, dependencies, or backend.

UI owns this independent reusable presentation contract. Catalog/source searches
found only commit navigation's use of the hook, not a global preference owner;
the small requirement/design pair records existing behavior. No ADR is needed.

## ASCII UI preview

UI-01: Existing Changes diff header and expanded Review, after a file-header
mode action (AC-UI-DIFF-PRESENTATION-001.1 and .3).

```text
Before: [Topbar: Split]   [file: Unified]  current: split
After:  [Topbar: Unified] [file: Unified]  current: split
Next:   [Topbar: Split]   [file: Split]    current: unified
```

Labels describe the next action, not new product copy. Keep existing grouping
and localized labels. Desktop file headers retain their mode button; phone file
menus retain their existing actions without inventing a mode entry. Phone opens
Changes from bottom navigation and Review from its Review action. The Review
topbar reflects the same preference; existing panel overflow remains available
where rendered. Fixed headers and existing internal diff scrolling remain.
Spacing above is illustrative; no geometry, navigation, or copy redesign.

## Tests and E2E

`hooks/use-global-view-mode.test.tsx` provides causal RED through actual Review
consumers and deferred wire transport, two-direction/next-click, raw preference,
remount, storage-event and ordinary-toggle coverage (AC .1-.3). Existing
`use-session-file-reviews.test.tsx` protects independent reviewed-file behavior.
`diff-view-mode.spec.ts` and `mobile-diff-view-mode.spec.ts` use isolated managed
runtime data and actual task Changes/Review controls. Component-only proof must
be labelled if browser infrastructure prevents execution.

## Work orders

- [x] [Task 01: Synchronize existing controls](task-01-synchronize-controls.md)

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/use-global-view-mode.test.tsx hooks/use-session-file-reviews.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/review/diff-view-mode.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome -- e2e/tests/review/mobile-diff-view-mode.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-global-view-mode.test.tsx components/review/review-dialog.tsx components/task/task-changes-panel.tsx e2e/tests/review/diff-view-mode-helpers.ts e2e/tests/review/diff-view-mode.spec.ts e2e/tests/review/mobile-diff-view-mode.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node - <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', 'bb7839fbd9c381e1883a9cd08b76b34b41b17698'], { encoding: 'utf8' }).trim().split('\n'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
])].filter(Boolean);
const fileContents = Object.fromEntries(paths.filter(p => p.startsWith('docs/')).map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles: paths.map(filename => ({ filename, status: 'modified' })), fileContents });
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
NODE
git diff --check
```

Use Node 24 with `NODE_OPTIONS=--max-old-space-size=8192`; build with
`GOMAXPROCS=2 GOFLAGS=-p=2`. Keep normal resource guards and local retries zero.
Docs coverage uses the repository validator locally without GitHub writes.
Fresh worktree installation:
`(cd apps && pnpm install --frozen-lockfile)`.

## Verification results

Completed the two hook adapters; the existing global hook and protocol are
unchanged. Production diff: 12 insertions and 22 deletions across two files.

- Permanent causal RED: 6 expected behavior failures and 4 passing controls,
  through actual Review dialog/file toolbar, real global hook/providers/protocol,
  deferred transport only. Initial wrong-working-directory/no-test invocation was
  setup-only and is not RED.
- GREEN: 39 tests pass across the 10 mode cases and 29 existing file-review cases.
  Both mode directions, next click, mounted readers, raw saved mode, remount,
  invalid/missing defaults, storage events, and independent word-wrap state pass.
- Managed production build and actual desktop/phone flows pass with strict WS
  accounting, one worker/shard, retries zero. Desktop covers file-header changes
  in Changes and Review; both viewports cover Review changing the mode while the
  actual Changes consumer remains mounted underneath, then its next activation.
  Phone uses its native Changes sheet and existing overflow controls. No
  component-only or data-only mobile exception is used.
- Two desktop selector/close-transition setup failures were corrected in test
  code, separately from causal RED. No production changes followed the first GREEN.
  Capture runs were additionally used to inspect stable desktop/phone screenshots.
- Node24 bounded 8GiB typecheck, focused ESLint, i18n check/ratchet, catalog,
  full specification lint, specification-linter tests, local PR-docs coverage,
  and whitespace checks pass. ESLint's initial duplicate-string warning was
  corrected with a test constant before the clean run.
- Public docs assessment: existing `docs/public/sessions-and-review.md` already
  describes mode switching and independent wrap/review actions. Procedures,
  labels, defaults and navigation do not change, so no public edit is required.

Raw logs/screenshots, full patch and final hook/process receipt are retained
outside checkout at the task's `loop-receipts/attempt19` directory. Browser
screenshots match UI-01's existing grouping; no layout changes were required.
All owned handles are reconciled to terminal in the final receipt. The mandatory
clean committed parent-validation checkpoint precedes any push/PR.


## Risks and delivery boundary

A stale production build can mask wiring errors; managed E2E rebuilds first.
No operator runtime/shared seed or foreign processes may be changed. Public docs
assessment: `sessions-and-review.md` already describes switching unified/split;
no user procedure or terminology changes, so no public edit is expected.

All controller-to-child messages explicitly use `delivery_mode=interrupt`;
reconcile existing handles after an interruption before new runs. Upward parent
messages use `delivery_mode=queued`. No delegation or extra tasks/sessions.
Stop at clean committed `ready_for_parent_validation`; parent independently
reviews/reruns before separate delivery authorization. No push or PR this turn.

## Parent validation correction: fixture isolation

The parent accepted the production diff and independently reran all 39 focused
tests, then identified that the browser fixture omitted `executor_profile_id`.
The worker-scoped seed exposes `worktreeExecutorProfileId` for explicit task
isolation; ApiClient forwards no default. Runtime executor resolution can use the
workspace default, so source ownership was not established by cleanup alone.
Add a path assertion before fixture writes, prove its failure with the omitted
profile, then explicitly select the seeded worktree profile. Capture seed HEAD
and complete porcelain status before task creation and assert both unchanged
after fixture cleanup. Keep production files byte-identical to `a9844280da07`.
Rerun the existing managed desktop/phone scenarios with one shard/worker and
zero retries, scoped ESLint/typecheck and documentation/diff checks, then make a
normal hooked commit and stop for parent validation. No push/PR is authorized.

Correction results: ownership RED confirmed the omitted selection returned the
shared source path `/tmp/kandev-e2e-0-pvoCYt/repos/e2e-repo`; the guard failed
before any file write/staging. Explicit worktree selection then passed both real
managed flows: desktop 1/1 (12.1s total) and phone 1/1 (14.6s total), strict WS,
one worker/shard, retries zero. Both assert real-path separation, unchanged seed
HEAD/full porcelain status while the task file is staged, and unchanged seed
HEAD/status after cleanup. Scoped ESLint, Node24/8GiB typecheck/catalog, full
specification lint, local PR documentation coverage and diff checks pass.
Production bytes remain identical to `a9844280da07`. Correction receipts live
outside checkout under `loop-receipts/attempt19/fixture-isolation/`. The prior
checkpoint's E2E proof covered independent managed runtimes, but did not prove
per-task source isolation; this correction supplies that missing evidence.
