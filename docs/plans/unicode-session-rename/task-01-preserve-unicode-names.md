---
id: "01-preserve-unicode-names"
title: "Preserve Unicode session names"
wave: 1
status: done
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002
acceptance_criteria:
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.5
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.6
system_design:
  - ../../specs/ui/system-design/task-agent-tab-reconciliation.md
---

# Task 01: Preserve Unicode Session Names

## Outcome and ownership

Implement AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.5 and .6 under
REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002 and the
[owning design](../../specs/ui/system-design/task-agent-tab-reconciliation.md).
No dependencies. Own use-session-rename.ts, tab-rename-input.tsx, session-tab.tsx,
preview-session-tabs.tsx, focused integration tests and session rename E2E,
phone Unicode picker E2E, and this package/owning documentation. No backend,
terminal normalization, ordering, lifecycle, shared runtime, or sibling edits.

## Acceptance

1. Unchanged legal 120-code-point names (non-BMP/mixed/BMP/ASCII) emit no request;
   changed names retain complete code points, trim/reset and cap at 120; actual
   WS wrapper payload and store agree, failures leave the original store.
2. Real input Enter/blur commits once, Escape cancels, default terminal UTF-16
   maxLength remains. Shipped desktop controls accept the full legal Unicode
   boundary and preserve names after reload.
3. Native phone picker displays canonical Unicode names and switches sessions;
   document that no phone/tablet session rename control is shipped. Selection
   shall emit no lifecycle request or error toast; absence of Rename is source-
   traced, without opening unrelated lifecycle menus.

## ASCII UI previews

UI-01 and UI-02 from [plan](plan.md#ascii-ui-previews):

```text
Desktop: [ Agent tab: <current name selected> ]
         Enter / outside: commit; Escape: cancel
Phone:   [ Current session v ] -> Sessions
         [ * <canonical name> ] [ <other name> ... ]
```

Preserve structure, focus, scroll owners and actions. No geometry change.

## Verification

From repository root, independently rooted commands:

```sh
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/use-session-rename.test.tsx components/task/tab-rename-input.test.tsx components/task/preview-session-tabs.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/use-session-rename.ts components/task/use-session-rename.test.tsx components/task/tab-rename-input.tsx components/task/tab-rename-input.test.tsx components/task/session-tab.tsx components/task/preview-session-tabs.tsx e2e/tests/session/session-tab-rename.spec.ts e2e/tests/session/mobile-session-unicode-name.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
GOMAXPROCS=2 GOFLAGS=-p=2 sh -c 'cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/session-tab-rename.spec.ts -- --retries=0'
GOMAXPROCS=2 GOFLAGS=-p=2 sh -c 'cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/session/mobile-session-unicode-name.spec.ts -- --retries=0'
(cd apps && cd .. && node --test scripts/validate-public-docs.test.mjs)
(cd apps && cd .. && node scripts/validate-public-docs.mjs)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
# Local invocation of the repository PR-documentation validator:
(cd apps && pnpm exec node - <<'JS'
const fs = require("node:fs"), cp = require("node:child_process");
process.chdir(cp.execFileSync("git", ["rev-parse", "--show-toplevel"], {encoding:"utf8"}).trim());
const paths = [...new Set([
  ...cp.execFileSync("git", ["diff", "--name-only", "32575f4c88fe3c734e23fec1253792704cac7480"], {encoding:"utf8"}).trim().split("\n"),
  ...cp.execFileSync("git", ["ls-files", "--others", "--exclude-standard"], {encoding:"utf8"}).trim().split("\n"),
].filter(Boolean))];
const docs = ["docs/plans/unicode-session-rename/plan.md",
  "docs/plans/unicode-session-rename/task-01-preserve-unicode-names.md",
  "docs/specs/ui/requirements/task-agent-tab-reconciliation.md",
  "docs/specs/ui/system-design/task-agent-tab-reconciliation.md"];
const result = require(process.cwd()+"/.github/scripts/pr-docs.cjs").validateCoverage({
  changedFiles: paths.map(filename => ({filename, status:"modified", additions:1, changes:1})),
  fileContents: Object.fromEntries(docs.map(p => [p,fs.readFileSync(p,"utf8")])),
});
console.log(JSON.stringify(result)); process.exitCode = result.ok ? 0 : 1;
JS
)

git diff --check
```

The preflight uses the existing repository validator directly; no standalone
preflight CLI exists. Review assertion coverage against acceptance; no artificial
unit-only percentage gate. Retain external logs and every session handle through
terminal closure. Refresh duplicate check, commit with normal hooks and capture
full receipt, then report exact head/diff to parent. No push/PR.

## Results

Completed in the authorized single session on 2026-10-22. No delegation.

- Frozen install: terminal 0, once in this worktree.
- RED unit command: 9 intended behavior failures / 9 passing controls before
  production changes (18 tests). The earlier parent diagnostic independently
  reproduced the two unchanged-name failures. No fixture/import error counted.
- RED desktop: initial observer scaffold failed parsing terminal WS frames;
  restricted capture to gateway `/ws`, reran unchanged production build, and
  observed the intended failure: unchanged 120 emoji emitted 60 emoji.
- GREEN focused unit command (including preview-session-tabs.test.tsx):
  3 files, 57 tests passed, final run after the last fixture change. Typed
  TaskSession fixture uses no incomplete cast. It covers legal unchanged
  ASCII/BMP/non-BMP/mixed names, overlong edits, combining code points,
  draft limiting without trim, API/store equality, reset, Enter, blur, cancel,
  failure, and shared default native maxLength. Preview consumer suite passed.
- Scoped ESLint with `--max-warnings 0`: passed, zero warnings.
- Typecheck: passed with CI's documented 8 GiB heap setting. Initial default
  4 GiB attempt exhausted V8 heap (134). The first 8 GiB attempt found excess
  loaded/loading fields in the diagnostic-derived test fixture (TS2353, 2);
  removed those fields and reran to terminal 0. No guards were bypassed.
- Fresh managed production desktop build/browser: 2 passed, zero retries,
  one worker/shard, strict WS accounting. Existing native external-focus and
  double-click isolation checks passed. Unicode control exercised actual
  SessionTab, real API/backend, observed gateway payloads and persisted reload.
- Original phone run at de5ba1ff: 1 test passed but its screenshot contained
  a restart failure toast. That result did not establish a clean interaction;
  the parent checkpoint invalidated the browser-menu evidence. See the correction
  below. No mobile rename or clean native-menu claim is made.
- Catalog validation: 396 decisions / 1693 specifications validated. Full
  specification lint passed. Repository PR-documentation validator preflight
  (the exact inline command above) returned covered / no errors.
- Diff whitespace and plan path/status checks passed. Public documentation
  clarifies the existing desktop-only action and canonical code-point limit;
  no UI copy or translation changes.

Acceptance coverage: 002.5 is proved at the actual input/committer/API/store
boundary and the shipped desktop control; 002.6 is proved by request/store
assertions, focus/completion controls, failed transport, and native browser
Enter/blur/Escape. Phone proof covers existing 002.4 display/navigation only.
No backend semantics, delayed delivery ordering, lifecycle, permission or
shared seed changes. Disposable E2E runtimes were managed by the runner and
terminated normally. Source and browser proof do not claim a prior user-runtime
incident or physical-device OS behavior.

Final duplicate refresh: 69 open PRs; PR1512 head a8655ad92faa050a68140a904500fc99ebc0a04b
and PR4013 head 37bfa1ec917dd2e8aaff1c04b6642065fbd03303 retain existing rename
caps and have no Unicode correction. PR3968 has no affected target files.
Main advanced from base 32575f4c88fe3c734e23fec1253792704cac7480 to
c971923bb9505a0030de2539884ebf35457729ab (chat task mentions, one disjoint
commit, including dependency changes). This package stays on the original
base for parent validation. Combined-current-main validation belongs to the
separately authorized delivery phase.

Receipts are outside the checkout at `/tmp/kandev-attempt15-unicode/`, including
RED/GREEN logs, exact PR snapshots, preflight and phone screenshot. Commit and
process-closure receipt is supplied to the parent after the normal commit.
Stop: ready_for_parent_validation, no push or PR.

## Parent checkpoint: phone evidence correction

The parent independently reran the 57 focused tests at de5ba1ff and found a
restart error toast in the phone screenshot. New gateway assertions reproduced
a separate native Session-actions trigger defect in three terminal diagnostic
runs: no lifecycle request before or after touching the session row, then
`session.stop` and `session.launch` immediately after the Session actions tap,
before Escape. Event capture shows pointerdown/up on the trigger followed by
a click on the newly rendered Restart session item. The fixture's seeded
WAITING_FOR_INPUT session has no executor record, explaining the failed restart
response; that fixture does not cause the unintended action.

`SessionActionsMenu` uses Radix's dropdown trigger, which opens on pointerdown;
the menu's action wiring invokes `useSessionRestart`. This is a separate touch
menu/retargeting boundary, with no connection to Unicode limiting or this
package's production diff. Exact logs and the instrumented diagnostic are
preserved outside the checkout in
`/tmp/kandev-attempt15-unicode/followup/phone-red.log`, `phone-phases.log`,
`phone-gesture.log`, and `menu-retarget-diagnostic.spec.ts`. The original faulty
screenshot remains preserved. The defect is known and remains unfixed here.

The parent explicitly limited remediation to the Unicode value: canonical
labels and real touch session selection. The permanent scenario therefore
omits the unrelated lifecycle menu, keeps all taps as real touch actions,
observes gateway traffic from before navigation, positively verifies a real
subscription to the selected session, then requires zero lifecycle mutations
and no toast. The unchanged source trace establishes no phone/tablet Rename
entry point; browser-menu absence is no longer claimed. No click substitution,
error dismissal, muted errors, mocked actions, or product repair is used.

Follow-up results:

- Native phone scenario: 1 passed (9.3s), zero retries, strict WS accounting,
  one worker/shard, unchanged production bundle. It asserts exact canonical
  labels, selected row, no horizontal overflow, real subscription, zero stop,
  launch, resume, restart, delete or primary-change requests, and zero toasts.
  The settled phone picker screenshot is `followup/phone-clean-value.png`.
- Focused three-suite unit command: 57 passed (3.25s). This fresh run supplements
  the original-head and independent parent evidence; no production code changed.
- Changed E2E scoped ESLint with zero warnings: passed.
- Public-doc validator tests: 63 passed. Validator: 54 published pages valid.
- Typecheck: passed with CI's documented 8 GiB heap. Final catalog/full spec
  lint and repository PR-doc preflight passed. Public validator tests require
  repository-root working directory; the intermediate apps-directory invocation
  failed on fixture path lookup, then the correctly rooted command passed.
  The normal follow-up hook receipt accompanies the parent handoff.
- Refreshed inventory: 69 open PRs, overlapping PR1512/4013 heads unchanged,
  neither touches the session limiter/shared input. Main is now
  d07dcea274d967ccc28d9bbc224196b531315207: chat mentions plus release-upgrade
  tests/runner work. No Unicode production path overlap. Later delivery still
  requires dependency refresh and combined-current-main validation.

The production Unicode fix is unchanged. Parent retains the separate touch-menu
candidate; no third task, session, agent, push or PR is created by this package.
