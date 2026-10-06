---
id: "01-refresh-success-feedback"
title: "Refresh clipboard success feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CLIPBOARD-FEEDBACK-001
acceptance_criteria:
  - AC-UI-CLIPBOARD-FEEDBACK-001.1
  - AC-UI-CLIPBOARD-FEEDBACK-001.2
  - AC-UI-CLIPBOARD-FEEDBACK-001.3
  - AC-UI-CLIPBOARD-FEEDBACK-001.4
  - AC-UI-CLIPBOARD-FEEDBACK-001.5
  - AC-UI-CLIPBOARD-FEEDBACK-001.6
  - AC-UI-CLIPBOARD-FEEDBACK-001.7
system_design:
  - ../../specs/ui/system-design/clipboard-feedback.md
---

# Task 01: Refresh Success Feedback

## Summary

Replace the existing hook-owned expiration on each acknowledged success and
release its currently scheduled timer on unmount. Prove the timing through
meaningful direct controls and the real localized workflow export dialog.
ROOT admitted implementation after the actual design END and verified the four
reviewed SHA256 values. Status is now in progress in the same primary/profile.

## In scope

- `useCopyToClipboard` timer ownership, replacement and scheduled cleanup only.
- Permanent regression tests using the real hook and clipboard utility, with
  only native clipboard methods and fake time substituted.
- A real rendered `WorkflowExportDialog` with actual Radix and translations,
  positive visible label/content evidence and exact native/fallback copy values.
- Existing native/fallback/focus controls and exact affected verification below.

## Out of scope

- Replaying, modifying or removing accepted original ROOT31773 proof.
- Request-order arbitration, cancellation, global clipboard state, async
  unmount admission/versioning, formats/persistence or utility focus refactoring.
- Consumer production changes, copy labels, layout, browser/build/E2E checks
  absent causal need, generated/unrelated fixture repair, broad local review.
- Any installation, production/permanent test edits, product checks, hooks,
  commit, PR, or merge during the current design-only turn.

## Acceptance

1. New default/custom repeated-success regressions fail causally on the
   baseline at the earlier deadline, while single-success/total-failure controls
   establish a working fixture. After the minimal hook fix they pass through
   the latest acknowledgement's full duration with exact supplied text intact.
2. Pending/failure/completion-order/duration-change/retained-callback/zero-duration
   and independent-instance controls preserve `001.2` through `001.6`. Unmount
   releases the currently scheduled timer after replacement, with another
   instance's deadline unaffected; no universal in-flight lifetime claim.
3. Real dialog activation proves localized Copy -> Copied -> Copy and visible
   content through both deadlines, native distinct text and real dialog-local
   fallback/focus. Real hook/utility/Radix/translation mocks, source assertions
   or copied predicates cannot substitute for these observations. All affected
   verification passes with actual joined handles/groups gone.

## UI-01: Existing workflow export feedback

Same shared temporal state as the [full plan preview](plan.md#ui-01-existing-workflow-export-feedback):

```text
[ existing read-only YAML content ]
[ Close ] [ Copy ] -> [ Close ] [ Copied ]
two successes at 0/1000 ms:
2000 ms: [ Copied ]; 3000 ms: [ Copy ]
```

Maps to `001.1` and `001.7`. Existing desktop and phone composition is the
boundary; pure state/data mobile exception applies. No geometry or real-browser
clipboard claim is inferred from happy-dom component evidence.

## Test procedure

Use `/tdd` after admission. Extend `hooks/use-copy-to-clipboard.test.ts`; add
`components/settings/workflow-export-dialog.test.tsx`. Keep all suites on the
configured `browser-locales` project with its real i18n setup and real Radix.

For durations 2000 and 1000 ms, acknowledge distinct text at 0 and duration/2;
observe feedback immediately before and at the old deadline, just before the
new deadline, and at expiry. Include a pending write with no premature success.
Resolve deferred native promises out of request order to prove completion order.
Fail both native and fallback during an existing successful deadline and prove
no early reset or extension. Rerender duration without copying; then verify a
new callback uses the new duration while a retained callback acknowledged later
uses its earlier capture. Include zero-duration and independent instances.

Scheduled cleanup should observe the fake scheduler's pending work in an
isolated hook fixture, including after replacement and with another live hook;
do not assert internal ref contents or number of `clearTimeout` calls. Clean up
React fixtures before restoring real time and original clipboard descriptors.
No in-flight unmount test is needed or promised.

In the real dialog, query the actual accessible button and visible textarea
content. Copy content A, advance 1000 ms, rerender with content B and copy again
from the existing Copied button; verify both distinct native write values and
positive Copied visibility at 2000 ms, then Copy at 3000 ms. Resolve native
acknowledgements within React `act`; do not use a mocked hook or utility.
For fallback, explicitly focus the real dialog copy button. At the native
`execCommand` substitution inspect the actual selected textarea's value and
dialog containment; return success, then verify textarea removal, restored
button focus and actual Copied label. Keep ordinary unavailable/rejected-native
and total-failure controls in the hook/utility suites.

The RED run must reach expected behavioral assertions, not import/transport or
fixture failures. Record failing names and actual exit. After only the minimal
hook change, rerun the same selection for GREEN. Affected-only causal fixture or
lint corrections are allowed; resource/timeout/transport/unknown/out-of-scope
failure means checkpoint ROOT, no automatic retry or weakened assertions.

## Verification

These commands define the targeted implementation checks. ROOT admitted their
execution after reviewing the four design artifacts; results are recorded below.
Launch each serially with an upfront native PID/group/start/absolute-cutoff/log
receipt. Actually wait/join every returned native handle and prove the owned
group gone before the next command. Keep logs. Vitest gets 120 s plus kill-after
10 s, one worker, no file parallelism, and a 4 GiB Node ceiling.

Use `/bin/bash` with login disabled. The default login zsh strips Node from
PATH on this executor. In that implementation shell only, prepend the existing
Node 24 binary directory; do not change global settings or install a runtime:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
```

Dependencies are absent at design audit. Only after ROOT explicitly grants an
exclusive lease, if still absent, permit exactly one pinned install from root:

```bash
timeout --signal=TERM --kill-after=10s 120s corepack pnpm@9.15.9 --dir apps install --frozen-lockfile
```

No reinstall if present; no automatic install retry. After that prerequisite,
run the following command once for RED and again after the causal fix for GREEN
from repository root; preserve ordinary utility/focus controls in both runs:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism hooks/use-copy-to-clipboard.test.ts lib/utils/copy-to-clipboard.test.ts components/settings/workflow-export-dialog.test.tsx)
```

After GREEN, run once each. ROOT admitted the normal project typecheck
prehook to prepare its existing ignored assets; no build or source/generator
change is authorized:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 60s corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/use-copy-to-clipboard.ts hooks/use-copy-to-clipboard.test.ts components/settings/workflow-export-dialog.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 60s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 60s corepack pnpm@9.15.9 run i18n:ratchet)
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 60s git diff --check
```

Use the repository documentation validator directly for the actual four
artifacts and affected hook scope (no network/PR needed):

```bash
timeout --signal=TERM --kill-after=10s 60s node - <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/ui/requirements/clipboard-feedback.md',
  'docs/specs/ui/system-design/clipboard-feedback.md',
  'docs/plans/clipboard-success-feedback/plan.md',
  'docs/plans/clipboard-success-feedback/task-01-refresh-success-feedback.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.concat('apps/web/hooks/use-copy-to-clipboard.ts')
  .map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
JS
```

This validates the named implementation scope, not a hosted current-head PR
verdict. Design's corrected invocation used only the actual four-document diff
and returned `exempt` with empty errors; it did not execute this future
production-scope command. Check actual diff inventory separately. No generated-fixture repair for
a pre-existing typecheck blocker; checkpoint ROOT with exact evidence.

## Files likely touched

- `apps/web/hooks/use-copy-to-clipboard.ts` (sole production file).
- `apps/web/hooks/use-copy-to-clipboard.test.ts`.
- `apps/web/components/settings/workflow-export-dialog.test.tsx` (new).
- `apps/web/lib/utils/copy-to-clipboard.test.ts` only if needed for affected
  causal controls; utility production file stays unchanged.
- The four design artifacts for accurate lifecycle, status and results.

## Dependencies

None between work orders. Later explicit SAME-primary implementation admission
and, only if needed, ROOT exclusive dependency-install lease are prerequisites.

## Risks

- Setting true again does not change the boolean; renewal belongs in the
  acknowledged success path, not a copied-state effect.
- Duration-triggered cleanup or a latest-duration ref changes captured callback
  semantics. Request counters change the accepted completion-order contract.
- Radix fallback/focus and async timing fixtures must remain real. Do not mock
  primitives or relax assertions to get GREEN.
- Already-awaiting success after unmount remains a documented residual.

## Parallelism

`sequential`; same primary/profile, no delegates or new sessions/tabs.

## Inputs

- [Requirement and all seven criteria](../../specs/ui/requirements/clipboard-feedback.md).
- [Timer and consumer design](../../specs/ui/system-design/clipboard-feedback.md).
- Existing hook/utility tests, `workflow-export-dialog.tsx`, and real locale
  setup in `vitest.setup.ts` / `vitest.setup.locales.ts`.
- [Plan's accepted evidence and delivery gates](plan.md#baseline-and-evidence),
  plus the exact versioned Kandev task plan and ROOT instructions.

## Results

Done: minimal hook timer repair with real hook/utility/Radix/i18n proof. Default
and custom renewal, completion order, captured duration, pending/failed writes,
zero-duration behavior, independent instances and current scheduled cleanup pass.
The post-unmount in-flight residual remains explicitly excluded.

Actual commands ran from repository root through the retained native supervisor
with the same listed working directories, package pin, absolute time limits and
10 s termination grace. Full argv/start/cutoff/log/exit/join/group evidence is in
`/tmp/kandev-clipboard-design-231344ee-<label>.json` and `.log`.

| Label / exact command selection | Native PID/group | Exit / result |
| --- | --- | --- |
| `install-once`: `corepack pnpm@9.15.9 --dir apps install --frozen-lockfile` | 1722850 | 0; 1.968 s; single admitted install, 935 packages reused, no lockfile edits. |
| `permanent-red`: listed Vitest command, exact three suites | 1730494 | 1; 7.630 s; 7 causal failures / 19 passing controls / 26 total. |
| `permanent-green`: same exact selection after sole hook production edit | 1731597 | 0; 7.478 s; all 26 pass. |
| `affected-format`: Prettier on the three changed TS/TSX files | 1733359 | 0; hook unchanged, test formatting only. |
| `affected-eslint`: listed three-file command | 1733630 | 1; two test-group line-limit warnings; preserved. |
| `affected-format-lint-correction`: Prettier on regrouped hook tests | 1736093 | 0; no assertion change. |
| `affected-eslint-corrected`: listed three-file command | 1736266 | 0; no warnings. |
| `final-affected-green`: same three suites after test regrouping | 1736908 | 0; all 26 pass. |
| `normal-typecheck`: normal pinned `run typecheck` with Node 4 GiB | 1737322 | 2; TS2769 in new dialog tests' unsupported `exact` option; preserved. |
| `consumer-eslint-after-type-fix`: changed dialog test only | 1740972 | 0; exact string-name assertions retained. |
| `final-green-after-type-fix`: exact three affected suites | 1741514 | 0; 7.579 s; 18 hook + 4 utility + 4 rendered dialog tests pass. |
| `normal-typecheck-corrected`: normal pinned `run typecheck` | 1741854 | 0; 8.430 s; no TypeScript errors. |
| `i18n-check`: pinned `run i18n:check` | 1742216 | 0; 9.434 s; catalogs and all copy gates pass. |
| `i18n-ratchet`: pinned `run i18n:ratchet` | 1743138 | 0; 1.567 s; changed production copy clean. |
| `actual-implementation-coverage`: repository validator on actual seven-file diff | 1747156 | 0; `covered`, `requiresCoverage: true`, `errors: []`, owning design accepted. |
| `implementation-catalog-validation`: `python3 scripts/list-docs.py validate` | 1747913 | 0; 355 decisions / 1400 specifications. |
| `implementation-spec-linter-tests`: `python3 scripts/lint-spec-files.test.py` | 1747940 | 0; all 36 pass. |
| `implementation-spec-lint`: `python3 scripts/lint-spec-files.py --all` | 1747984 | 0; all specs pass. |

Each started process actually joined, with its group gone before the next command.
No assertion/race/timeout weakening, resource retry, broad product replay, build,
or browser/E2E launch occurred. Routine corrections touched only the causal new
test fixture. Real Radix emitted its existing missing-description warning during
RED; it was not suppressed or used to expand consumer production scope.

Pretypecheck prepared ignored `apps/web/generated/release-notes.json` (v0.97.0)
and `apps/web/generated/changelog.json` (117 entries) through the existing local
generators. Scripts/source/tsconfig/global settings/lockfiles remain unchanged.
The original design missing-Node startup failure remains separately preserved;
all execution package calls used pinned Corepack and the existing Node 24 PATH.

Requirement/design lifecycle is active/current; plan is implemented. Final
whitespace/inventory, normal active-hook commit, READY publication and hosted
exact-head evidence are recorded in the versioned Kandev task plan. Verified
merge and joined cleanup still gate platform completion; no merge grant exists.
