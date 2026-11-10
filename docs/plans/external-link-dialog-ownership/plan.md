---
created: 2026-10-22
status: done
requirements:
  - REQ-TASKS-TASK-ACTIONS-MENU-004
system_design:
  - ../../specs/tasks/system-design/task-actions-menu.md
legacy_specs: []
---

# Implementation Plan: External link dialog ownership

## Overview

Preserve a newer link draft when an earlier dismissed operation completes.
The source-proven cause is an unguarded close callback after awaited issue
lookup and task PATCH; the sidebar conditionally unmounts the old form but its
callback clears the parent's current target. User explicitly authorizes this
package followed by same-turn TDD; delivery remains separately gated.

## Scope

Own only external issue-link request/edit lifetime admission and faithful tests.
Exclude sibling mobile Rename, PR/MR forms, Query/cache migration, backend
cancellation, shared modal changes, API/schema/authorization, and provider transport.

## Technical approach

Follow the existing Tasks design's local opening identity plus keyed inner form.
Do not create a parallel spec or ADR: the existing vertical pair preserves the
complete bounded rule and rationale. Captured writes and latest-title lookup
remain admitted after dismissal. Current inline errors remain; detached errors
use application feedback. Commit ownership guards survive StrictMode.

| Provider | Existing lookup | Identity | Evidence |
| --- | --- | --- | --- |
| Jira | ticket GET then task PATCH | task/workspace/provider, captured key | real API component and desktop/phone fixtures |
| Linear | issue GET then task PATCH | same, captured identifier | existing provider test, replacement regression |
| Sentry | instance issue GET then task PATCH | same, captured selected instance | existing healthy-instance and normalization tests |

## ASCII UI preview

UI-01, existing desktop sidebar Link > Jira Ticket and phone Tasks picker >
Task actions > Link > Jira Ticket. Existing desktop composition:

```text
Before: old PATCH settles -> newer link dialog disappears
After:
+ Link Jira ticket -------+
| Ticket: APP-2           |
| [Cancel]         [Save] |
+------------------------+

UI-01 phone (existing short form):
+ Link Jira ticket -------+
| Ticket: APP-2           |
| [Save]                 |
| [Cancel]               |
+------------------------+
```

The phone retains its shipped picker and menu sheets and the same short link
form; this repair changes state admission only, not composition, geometry,
scrolling, safe areas, or touch handling. Draft and enabled actions survive the
old completion. A current failure shows its inline error. The drawing is
structural, not pixel geometry. Native taps must reach the shipped phone entry.

## Tests

`task-external-link-ownership.test.tsx` exercises actual SidebarLinkDialogs,
StateProvider, ToastProvider, coordinator and APIs with only fetch deferred.
Covers AC-TASKS-TASK-ACTIONS-MENU-004.7: same/different reopen; stale/current
success/failure; unmount; retained closed/open; task/provider/workspace replacement;
StrictMode and abandoned render ownership; captured original update/latest title.
Existing dialog tests own Sentry selection and title normalization.

## E2E tests

`external-link-ownership.spec.ts` (chromium) and
`mobile-external-link-ownership.spec.ts` (mobile-chrome) use the shipped sidebar
and task picker with real disposable backend and mock Jira fixtures. Delay only
the causal PATCH. Assert original update, preserved later draft, explicit later
Save/Cancel and focus. No real provider credentials or operator runtime changes.

## Work orders

- [x] [Task 01: Preserve current link](task-01-preserve-current-link.md)

## Verification commands

```bash
(cd apps/web && pnpm exec vitest run components/task/task-external-link-ownership.test.tsx components/task/task-external-link-dialog.test.tsx components/task/task-external-link-utils.test.ts --maxWorkers=1)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project chromium tests/task/external-link-ownership.spec.ts --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-external-link-ownership.spec.ts --retries=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/task-external-link-dialog.tsx components/task/task-external-link-ownership.test.tsx e2e/tests/task/*external-link-ownership*.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.test.py
python3 scripts/lint-spec-files.test.py
git diff --check 07ba3a68b31f837bac0f4df5a6b3fa6644f20051
```

Use Node 24. Frozen apps install precedes pnpm. One worker/shard, retries zero;
managed fixtures own disposable data and e2e mocks. The unique test workspace
uses the fixture repository and explicit Worktree executor profile; assert its
source HEAD/status unchanged and delete the workspace in finally. Public docs
validation: `node scripts/validate-public-docs.mjs` from repository root.
The docs coverage preflight uses the entire base diff and the changed owning
contracts/package (do not send the whole docs tree past its 4 MiB input bound):

```bash
node - <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = '07ba3a68b31f837bac0f4df5a6b3fa6644f20051';
const changed = cp.execFileSync('git', ['diff', '--name-only', base], { encoding: 'utf8' }).trim().split('\n');
const added = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n');
const paths = [...new Set([...changed, ...added])].filter(Boolean);
const result = validateCoverage({
  changedFiles: paths.map(filename => ({ filename, status: added.includes(filename) ? 'added' : 'modified', additions: 1, changes: 1 })),
  fileContents: Object.fromEntries(paths.filter(p => p.endsWith('.md')).map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
process.exitCode = result.ok ? 0 : 1;
JS
```


## Verification results

Completed locally on base `07ba3a68b31f837bac0f4df5a6b3fa6644f20051`.

- Causal component RED on unchanged production: 9 expected failures / 5 passing
  controls. Both same/different reopen failures lose the new textbox only after
  old PATCH settlement. Additional current lookup-failure and replacement-pending
  controls bring final GREEN to 35 passing tests across 3 files (7.51s).
- Fresh managed backend/e2e Vite/plugin build, then final shipped sidebar desktop:
  2 passing (15.0s); native-tap Pixel 5 phone picker: 2 passing (17.3s). One
  worker/shard, retries 0, strict WS accounting, `GOMAXPROCS=2 GOFLAGS=-p=2`.
  Confirmed original persisted title, exact new draft and focus, subsequent
  Save/Cancel, and persisted titles after reload. Disposable workspace deleted;
  fixture source HEAD/status unchanged. Explicit Worktree profile and mock
  integration fixtures only. Final phone screenshot inspected: healthy task
  page, focused APP-2 form, stacked Save/Cancel matching UI-01.
- Zero-warning scoped ESLint; Node 24.18.0 typecheck with 8 GiB heap cap; rich
  output catalog and full i18n checks pass. Discovery across chromium,
  mobile-chrome and containers reports zero errors (listing only).
- Catalog validates 396 decisions / 1701 specs; full spec lint passes; catalog
  tests 9 and spec-lint tests 36 pass. Full base diff docs coverage is `covered`
  for the one work order. Full diff whitespace check passes.
- Public docs assessment: task actions behavior is user-visible. Added a bounded
  recovery paragraph to the existing task/workflow how-to page; 54 published
  pages validate. No new public API, configuration, terminology or screenshots.
- Setup findings corrected before final proof: workflow initially had no steps
  (use simple template); task initially lacked a repository (use disposable
  fixture repository with explicit Worktree profile); Suspense fixture required
  an explicit never return type. These were fixture/static failures, not causal
  RED. No production fix was justified by them.
- Scope remains only external link lifetime admission. No ADR: existing owning
  requirement/design pair fully records this local constraint. No real external
  provider, physical-device, operator-incident, hosted-CI, merge or delivery claim.

Durable receipts are outside checkout at
`/home/jcfs/.kandev/tasks/keep-newer-external_f3mmjyn4/receipts/`:
`red.log`, `green-final.log`, `browser-desktop-final.log`,
`browser-mobile-final.log`, `browser-{desktop,mobile}-results/`,
`typecheck-final.log`, `lint-final.log`, `i18n.log`, `catalog.log`,
`spec-lint.log`, `catalog-tests.log`, `spec-tests.log`, `docs-coverage.log`,
`public-docs.log`, `discovery.json`, and `build-inputs.sha256`.

Main and open-PR overlap evidence refreshed: main still equals the base; 69
open PRs. Actual paginated relevant patches for 1512 are unchanged Query/cache
work with no lifetime guard; 4517/4444 have no matching target patches. This is
bounded target inspection, not an unrestricted review of those PRs.


## Risks

Identity must be allocated by commit, not render; a boolean that reactivates on
reopen could admit stale work. Failure feedback must survive disposal without
contaminating a later form. Browser environment availability must be recorded.


## Review remediation: portable fixture ownership guard

The aggregate review identified that the E2E source-root assertion assumed
`/tmp`, while `backend.ts` creates `backend.tmpDir` beneath `os.tmpdir()`.
A managed same-task desktop run with an owned custom `TMPDIR` failed exactly at
that assertion before any repository/task mutation. The helper now derives its
expected prefix from the same system temporary-directory producer.

Focused custom-TMPDIR desktop and native-tap phone scenarios pass (2 each,
retries 0, one worker), retaining original writes and later Save/Cancel checks.
Scoped ESLint and Node 24 typecheck with the 8 GiB cap pass. Production form
source and rendered UI are unchanged, so the existing production-build proof,
35 focused tests, and published captures remain applicable.

Reproduce with an owned temporary directory:

```bash
(cd apps/web && TMPDIR=/path/to/owned/temp GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --no-build --shards 1 --project chromium tests/task/external-link-ownership.spec.ts --retries=0)
(cd apps/web && TMPDIR=/path/to/owned/temp GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-external-link-ownership.spec.ts --retries=0)
```

The configured root must exist and belong to the test invocation. The actual
run's root and causal RED/GREEN/static receipts are recorded outside checkout
in `receipts/custom-tmp-root.txt` and `receipts/portability-*` under this task's
working directory. Post-push CI/review and merge evidence remain external and
pending until the final head's checks and dispositions settle.
