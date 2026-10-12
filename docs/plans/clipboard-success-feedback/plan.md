---
created: 2026-10-06
status: implemented
requirements:
  - REQ-UI-CLIPBOARD-FEEDBACK-001
system_design:
  - ../../specs/ui/system-design/clipboard-feedback.md
legacy_specs: []
---

# Implementation Plan: Clipboard Success Feedback

## Overview

Renew the shared copied indicator from every successful clipboard
acknowledgement. One sequential work order adds causal hook and rendered
consumer regressions, replaces the hook-owned timer, and verifies compatibility.
The design turn ended before ROOT review. ROOT subsequently released
implementation in the same primary/profile with an exclusive LOCAL-HEAVY
lease. The hook-only repair and targeted checks are implemented; publication
and hosted verification are tracked in the versioned task plan. MERGE remains
unauthorized until a separate ROOT interrupt.

## Scope

### In scope

- The reusable [clipboard feedback requirement](../../specs/ui/requirements/clipboard-feedback.md)
  and its paired design, without changing path or transport ownership.
- One hook-owned scheduled expiration per mounted instance, replacement on
  success, and cleanup of the currently scheduled expiration.
- Direct causal timing controls and real `WorkflowExportDialog` rendered
  integration, including localized visible labels and exact copied content.

### Out of scope

- Transport cancellation, request-order arbitration, post-unmount async
  admission/versioning, and a global clipboard framework.
- Clipboard formats/persistence, utility focus refactoring, new labels/layout,
  consumer production changes, browser/build/E2E work absent causal need.
- Dependency installation, product checks, permanent tests or production edits
  during design. No generated or unrelated fixture repair.

## Baseline and evidence

Actual source audit on 2026-10-06 found worktree HEAD and remote main at
`30782135f3dfe1c532244969f0c2c08e2c54955f`, with a clean worktree.
`useCopyToClipboard` still creates an independent timeout after each success.
The earlier timeout can clear copied state before the latest success has had
its full duration. The utility itself continues to preserve exact text and
restore focus after its dialog-local DOM fallback.

Source blobs match the accepted ROOT proof: hook
`165daa1ee31234c69c96f952681056af093327cb`, utility
`85f47e99f86980f8eb9068643bf87c0c93941d71`.
The accepted original reproduction is read-only and must never be replayed,
modified, or removed:

- `/tmp/kandev-clipboard-feedback-duration-repro.test.ts`, SHA256
  `3fcce075e4f45d11c8097efedc8f0ec90299a96453444e14f89df2984c6d6946`.
- `/tmp/kandev-root-clipboard-feedback-proof-{receipt,classification,native}.json`
  and `.log`; original native 31773 actually joined with exit 1 in 5.185 s.
- ROOT accepted two causal duration failures (default 2000 ms/custom 1000 ms)
  and two passing controls for single success and failed native/actual fallback.
  The tests used the real hook/utility with native clipboard substitutions and
  fake time. Their original head was
  `670847f0a48cf36c14bdbd929a2fb96a3a565eea`.

This task audited current source without rerunning that proof. Original groups
are gone and original scratch cleanup was accepted by ROOT. New task-owned
design receipts/logs use `/tmp/kandev-clipboard-design-231344ee-*`.

## Technical approach

Apply the [timer design](../../specs/ui/system-design/clipboard-feedback.md#timer-ownership-and-completion)
inside `apps/web/hooks/use-copy-to-clipboard.ts`. Preserve the API and callback
duration capture. Success completion order, including a retained old callback,
determines timer replacement; failure leaves the previous timer untouched.
Keep `apps/web/lib/utils/copy-to-clipboard.ts` and consumer production files
unchanged. Clean up the current scheduled timer without extending scope into
in-flight completion suppression.

## UI-01: Existing workflow export feedback

Entry point: an open `WorkflowExportDialog`. Illustrative content and English
labels below represent existing localized UI. The only change is the duration
of the current successful state; the existing content and action arrangement
remain the test boundary.

```text
Export workflow
[ existing read-only YAML content ]
[ Close ] [ Copy ]

success at t=0; success at t=1000 ms:
t=2000 ms: [ Close ] [ Copied ]
t=3000 ms: [ Close ] [ Copy   ]
```

The same temporal outcome applies on phones within the existing dialog
composition. No new mobile composition is proposed. The mobile skill's pure
state/data exception applies; rendered component and hook tests cover the shared
feedback. The sketch maps to `AC-UI-CLIPBOARD-FEEDBACK-001.1` and `001.7`; it is
a state review aid, not visual/geometry proof.

## Tests

Planned test names are behavioral descriptions, not existing passing evidence.

| Criteria | Test file and planned evidence |
| --- | --- |
| `001.1`, `001.2` | `apps/web/hooks/use-copy-to-clipboard.test.ts`: repeated successes retain feedback beyond the first deadline (default/custom); ordinary single success expires; zero duration clears on the next timer turn. |
| `001.3` | Same hook suite: pending writes do not report success; an earlier-started write acknowledged last renews from its own completion. |
| `001.4` | Same hook suite: failure during prior success preserves that deadline; initial total failure never reports copied. |
| `001.5` | Same hook suite: duration rerender does not alter current deadline; next new callback uses new duration; retained old callback completes with its captured duration. |
| `001.6` | Same hook suite: independent instances expire independently; unmount releases the currently scheduled timer, including after replacement, without clearing another instance's timer. |
| `001.7` | `apps/web/components/settings/workflow-export-dialog.test.tsx`: real localized Copy/Copied and content remain visible through renewed duration; exact distinct values reach native writes; actual Radix-local fallback restores button focus. Existing `apps/web/lib/utils/copy-to-clipboard.test.ts` controls retain native preference, unavailable/rejected native fallback, textarea removal and focus. |

## E2E tests

No Playwright/build/browser launch is planned: shared state timing is the whole
change. The real rendered dialog integration is the smallest end-to-end slice
from user activation through hook and utility to visible feedback. This does
not claim real-browser clipboard permission or mobile geometry coverage.

## Documentation assessment

Public search found `docs/public/workflow-import-export.md`, which already tells
users to choose Copy to copy the export. It specifies no feedback duration.
No public command, config, schema, label, screenshot or user procedure changes;
internal specs/plans suffice. No system README boundary or migration change is
needed because UI already owns reusable presentation contracts. No ADR needed.

## Work orders

- [x] [Task 01: Refresh success feedback](task-01-refresh-success-feedback.md)

Only one work order, no dependencies, sequential in the same primary session.
No delegation, new session/tab, or model switch.

## Verification results

Implementation: 26 tests pass in the exact three affected suites after seven
causal RED failures and nineteen passing controls. Changed-file ESLint, normal
project typecheck, both i18n checks, actual implementation documentation
coverage, catalog validation, all 36 linter tests and full specification lint
passed. [Task 01 results](task-01-refresh-success-feedback.md#results) record
commands, corrections and native receipts. Hosted verification and merge are
pending externally. The following design results are historical.
Design checks are limited to 60 s each: catalog/validation, the existing
36-test spec-linter suite, full spec lint, real cross-reference/coverage preflight,
whitespace including new files, and exact four-file unstaged inventory. Retain
native PID/group/start/absolute-cutoff/log evidence and actually join every
handle, with groups gone.

Actual design results (all started commands exited 0, actually joined, groups
gone; no catalog/spec check was replayed):

| Check | Native PID/group | Actual result |
| --- | --- | --- |
| Initial clipboard catalog | 1684469 | Owner candidates found. |
| Current-main source/public-doc audit | 1685970 | HEAD equals remote main; original source blobs unchanged; dependencies absent. |
| Catalog validation | 1696977 | 355 decisions and 1400 specifications validated. |
| Spec-linter tests | 1697076 | All 36 tests passed. |
| Full spec lint | 1697257 | All specification files passed. |
| Python references/whitespace/inventory | 1700468 | Links/anchors, requirement/criterion/design references, whitespace, empty index and exact four untracked files passed. |
| UI clipboard owner catalog | 1701287 | Both new owning artifacts discoverable. |
| Clipboard decision catalog | 1701426 | No matching ADR. |
| One ROOT-authorized corrected coverage invocation | 1703859 | Actual document-only scope: `exempt`, `ok: true`, `requiresCoverage: false`, `errors: []`, `acceptedReferences: []`. |

The first coverage attempt failed before executing Node because login zsh's
PATH lacked `node`: `FileNotFoundError`, supervisor exit 1, no native coverage
handle returned and no coverage verdict. Preserve this setup failure. ROOT
authorized exactly one correction using the existing absolute Node 24.21.0
binary under `/bin/bash` with login disabled. All eight original command groups
were reconciled as gone before correction. Corrected native 1703859 joined with
exit 0 in 0.114 s; its group is gone. This exemption is not evidence of future
production coverage; Python reference auditing is separate evidence.

Receipts/logs, including start and absolute 60 s cutoffs, are retained under
`/tmp/kandev-clipboard-design-231344ee-*`. Final affected whitespace/inventory
and final handle reconciliation are recorded in the versioned Kandev task plan
at the design END checkpoint. No dependencies, source, harness, global setting,
or cache were changed for recovery. Implementation verification is recorded below and in Task 01; the historical
design-only exemption is not used as production coverage.

## Execution and delivery gates

The versioned Kandev task plan for task
`231344ee-68f0-4cf1-99f5-33583db1e211`, primary session
`e45176e3-e4ab-4f8a-931c-5e82ca1fca49`, preserves ROOT's complete operational
constraints and standing later delivery gates. Its system marker, identities,
user edits and question barriers must survive updates. After design validation,
checkpoint and actually END WAITING; design is not task completion.

After later implementation admission: one conditional pinned pnpm 9.15.9
frozen install only if dependencies are absent and ROOT grants an exclusive
lease. Product commands run serially with retained native handles/groups;
resource, timeout, transport, unknown or out-of-scope failures checkpoint ROOT
without automatic retry. Normal hooks and new Conventional Commits apply.

Later READY publication freezes the head except for actual corrective findings.
Use one original all-terminal `scripts/pr-await` collector (45 minutes, GNU
46-minute cutoff, kill-after 10 s); never replace/retry/duplicate without ROOT
admission and prior collector join. Require all six actual required contexts
and actual Backend/Frontend/E2E parent success, fresh exact-head governance and
resolver evidence, no actionable human/visible/hidden changes-requested review,
and substantive full current-head all-files review by authenticated configured
CodeRabbit App 347564. Automatic review suffices; only a real skip/gap can need
at most one full request. Inspect findings while CI runs. No bypass, main-only
rebase, synthetic merged test, weakened assertion/race/timeout, or unadmitted
hosted retry. ROOT alone admits bounded exact-failed-job/causal-dependency retry.

MERGE-ready must END before a separate ROOT serial merge interrupt and actual
static main/head/merge-tree/owned-blob/shared-contract review. Normal expected
head squash only. Independently verify REST/Git merge/parent/tree/owned blobs and
remote inclusion, join owned handles, prove groups gone, clean only owned
resources, and explicitly return the lease. Keep managed worktree/deps/logs.
ROOT independent proof precedes archive/refill/release of the original proof.
No foreign/shared mutation, indiscriminate kill/prune/ref/FETCH_HEAD changes.
Machine loss means unknown/no verdict until reconciliation. A full callback or
question queue means version-safe checkpoint and END WAITING, without retry or
new session. Task remains pending until verified merge and joined cleanup.

## Risks

- A boolean-driven effect misses renewal when feedback is already true.
- Resetting on invocation or using request order changes acknowledged-success
  semantics; using the latest duration breaks retained callback behavior.
- Native rejection may be successful DOM fallback. Test the final real outcome.
- Fake-clock teardown must unmount before restoring time and native descriptors;
  do not flush timers to hide a leak or suppress an actionable Radix failure.
- In-flight completion after unmount remains outside this repair's guarantee.


## Implementation results

The sole production edit is instance-local timer ownership in the shared hook.
No clipboard utility or consumer production file changed. Final suites contain
18 hook tests, 4 utility tests and 4 real rendered dialog tests, all passing.
Default/custom repeated-success, acknowledgement ordering and cleanup regressions
failed causally before the fix; the real English and Portuguese rendered
confirmation also expired early in RED. Native and actual Radix-local fallback
controls passed. In-flight success after unmount remains outside the contract.

The one pinned frozen install completed in 1.968 s. Final affected GREEN native
1741514 exited 0 in 7.579 s; corrected typecheck native 1741854 exited 0 in
8.430 s. The normal prehook prepared ignored `apps/web/generated/release-notes.json`
for v0.97.0 and `apps/web/generated/changelog.json` with 117 entries. No tracked
generator/script/tsconfig/lockfile changed, and no build ran.

Preserve original affected lint/typecheck failures: test-group callbacks exceeded
the line limit, then new role queries used an unsupported `exact` option. Smaller
test groups and removal of that redundant option corrected them without weakening
assertions. Final changed-file lint and both i18n checks passed. Actual seven-file
coverage native 1747156 returned `covered`, `requiresCoverage: true`, `errors: []`
and the owning design reference. Implementation catalog/spec36/full lint passed.
All native handles actually joined and groups were gone before the next command;
receipts and logs remain under `/tmp/kandev-clipboard-design-231344ee-*`.

Public documentation remains unchanged for the recorded concrete reason. No
Playwright/browser/build/screenshot work was added: this is state timing within
existing localized controls, under the approved mobile state/data exception.
Publication/CI/review, explicit LOCAL-HEAVY return, and separately authorized
merge remain in the versioned task plan; this implementation status is not a
platform task-completion or merge verdict.
