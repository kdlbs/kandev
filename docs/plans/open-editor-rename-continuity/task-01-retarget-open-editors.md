---
id: "01-retarget-open-editors"
title: "Retarget open editors after Files rename"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
  - REQ-UI-FILE-EDITOR-MUTATION-002
acceptance_criteria:
  - AC-UI-FILE-EDITOR-MUTATION-001.1
  - AC-UI-FILE-EDITOR-MUTATION-001.2
  - AC-UI-FILE-EDITOR-MUTATION-001.3
  - AC-UI-FILE-EDITOR-MUTATION-001.5
  - AC-UI-FILE-EDITOR-MUTATION-002.1
  - AC-UI-FILE-EDITOR-MUTATION-002.2
  - AC-UI-FILE-EDITOR-MUTATION-002.3
  - AC-UI-FILE-EDITOR-MUTATION-002.4
  - AC-UI-FILE-EDITOR-MUTATION-002.5
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
---

# Task 01: Retarget open editors after Files rename

## Summary

On accepted user-initiated Files rename or move, retarget the affected live
editors while preserving their buffers and interaction state. Keep rejected
operations and unrelated editors unchanged.

## In scope

- Shared successful rename delivery, repo/environment matching, descendant
  boundary handling, Dockview and tablet local-tab state and persistence.
- TDD regression for the qualified dirty editor flow, and bounded ownership,
  preview/pinned, failure, partial move, collision, reservation and scope tests.

## Out of scope

External rename inference, general stale-save ordering, backend or transport
changes, new copy, phone editing and new phone action controls.

## Acceptance

- An accepted file/folder rename or move retargets only matching live editors;
  dirty content, baseline, preview/pin, selected tab and save target survive.
- Fail preflight with existing rename-failure feedback for an already-open
  destination or overlapping pending reservation; block opens/restoration of
  reserved source/destination prefixes until settlement. Release only the
  originating token, leaving other paths and environments independent. Recheck
  at publication; preserve both buffers and report failure if an unexpected
  late destination appears after filesystem success. Failed
  operations and retired/replaced editors stay unchanged.
- Reopening the new path selects the same editor; saved tab restoration uses
  the new path, and existing mutation completion ownership stays intact.

## ASCII UI preview

UI-01, desktop Files rename, criteria .1-.5 (see [full preview](plan.md#ascii-ui-preview)):

```text
Before                       After success
Files    old.ts * [Save]     Files    new.ts * [Save -> new.ts]
old.ts   [dirty content]     new.ts   [dirty content]
```

UI-02, phone retains focused Files / read-only document composition:

```text
Files [row] [actions]  ->  Document [path] [Close] [content]
```

Phone lacks editable file and visible touch Rename actions, so real phone
rename/save evidence cannot be produced within this scope. Tablet uses Files
beside local editor tabs. Compare rendered desktop behavior with UI-01.

## Verification

Author independent tests; never read or copy ROOT's private qualification test.
Before implementation run the new focused desktop regression and targeted unit
cases to establish RED, recording actual failure. After implementation run:

```bash
(cd apps/web && pnpm exec vitest run hooks/file-rename-continuity.test.ts hooks/use-file-rename-continuity.test.tsx hooks/use-file-editors.open-action.test.tsx components/task/use-tablet-file-rename.test.tsx components/task/task-center-panel-file-tabs.test.ts components/task/task-center-panel-mutation-ownership.test.tsx components/task/task-center-panel-restoration.test.ts hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-operations.test.ts lib/state/dockview-file-state.test.ts components/task/file-browser-move-settlement.test.tsx)
(cd apps/web && pnpm exec vitest run hooks/use-file-rename-promotion.test.tsx hooks/file-rename-continuity.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/file-editor-rename-continuity.spec.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-file-operations.ts hooks/use-file-operations.test.ts lib/state/dockview-file-state.ts lib/state/dockview-file-state.test.ts components/task/task-center-panel-mutation-ownership.test.tsx e2e/tests/task/file-editor-rename-continuity.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Cover canonical tree/repo aliases, exact-segment descendants, cross-repo move
routing, environment-stable session, A-to-B-to-A visits, synchronous preflight,
pending source/destination open denial, earlier in-flight open publication,
deferred restoration and token cleanup. Reject a source panel without a stable
buffer before dispatch. Exercise both tablet
local adapter and Dockview replacement with a real API-compatible panel host:
old removal callback, group and tab order, globally/inactive-group selection,
preview promotion, instance preservation, no fallback loader fetch, and tab,
active-tab and debounced layout persistence.

Run each additional changed source/test file through scoped ESLint and each
additional changed unit suite through Vitest; record exact commands and results.
If user-facing copy changes, add all required translations and run `pnpm run
i18n:check` and `pnpm run i18n:ratchet` in `apps/web`. E2E runner rebuilds
before the browser run. Confirm test discovery and record actual results; do
not replay unrelated suites. Heavy checks require ROOT's resource grant.

## Files likely touched

- `apps/web/hooks/use-file-operations.ts`, `use-file-editors.ts`, the app-store
  reservation owner and their tests.
- `apps/web/lib/state/dockview-file-state.ts`, `dockview-panel-actions.ts`,
  editor hooks and nearby tests.
- `apps/web/components/task/task-center-panel.tsx`, `task-files-panel*.ts*`,
  `mobile/session-tablet-layout.tsx` and local ownership tests.
- `apps/web/e2e/tests/task/file-editor-rename-continuity.spec.ts`.

## Dependencies

None. Implementation requires a later explicit request.

## Risks

Dockview path-derived pinned IDs and removal cleanup can lose or duplicate a
buffer if handled incorrectly. Session and environment are not interchangeable.
Admission rejects a distinct open destination before transport; a narrow
reservation blocks racing opens until settlement. Exercise both aliases and
folder descendants without creating a general filesystem coordinator.
Phone's current native flow cannot perform the requested rename/save.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/file-editor-mutation-ownership.md)
  and [design](../../specs/ui/system-design/file-editor-mutation-ownership.md).
- Qualification evidence and current code; no private candidate source.

## Results

Completed. The independent desktop RED failed after the successful control Save
and rename because Save targeted the old path. A separate real-hook RED exposed
promotion while a rename was pending; a second RED proved that closing the
unrelated preview must not retire the pinned buffer. The corrected tests cover
both promotion variants, close/reopen, replacement host and preview, plus the
subsequent Save request at the new path.

- `pnpm exec vitest run` on the 11 focused owner/move suites: 89 passed; after
  the promotion fix, the three affected suites passed, then the promotion and
  owner controls passed 11 tests across two suites.
- Changed-file ESLint passed with `--max-warnings 0`; the promotion files were
  rechecked after their final changes. `pnpm run i18n:ratchet` passed.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed. An earlier
  default-heap attempt exhausted memory and did not produce a TypeScript verdict.
- Fresh managed `pnpm e2e:run --host --project chromium
  tests/task/file-editor-rename-continuity.spec.ts -- --retries=0` passed one
  test after the final source change, including a positive pre-rename Save,
  actual rename, retained dirty content and Save to the renamed file.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py
  --all`, both public-doc validators and `git diff --check` passed.
- A disposable tablet capture test passed one real rename and Save on the
  existing build. Its first attempt failed to locate the inline input because
  the row's visible text became an input; a stable path locator corrected it.
  The disposable test was removed after capturing the tablet view.
- Phone rename/save coverage is unavailable because its viewer is read-only
  and its visible touch menu has no Rename action. Tablet owner semantics are
  also covered by focused local-state tests.

Review follow-up reproduced two additional defects: an external absolute tab
blocked unrelated workspace renames, and a rename while another restored tab
was loading failed to persist the new path. The fixes require a known,
comparable workspace root to identify external paths and persist current tabs
only for the originating visit and host. Focused tests cover persisted readback,
retired owners and ambiguous or workspace-contained absolute paths. A managed
browser check also confirmed undo history can reset after rename; preserving it
is outside this change and is disclosed in the requirements and public guide.
Five affected Vitest suites passed 51 tests, changed-file ESLint and the 8 GiB
typecheck passed, and a fresh managed Chromium run passed the rename/save flow
with tracing enabled. The unrelated hosted executor-selector failure passed one
isolated local attempt with retries disabled; the hosted result remains failed.

## Save completion observation in CI (2026-10-27)

PR #4534 head `3e4a5a3aff7f` has one failing browser attempt: the final disk
read after Save throws ENOENT. The failed result has screenshot/context only.
The retained trace belongs to its successful retry. The rename response and
pre-save destination content checks passed, so that path was valid before Save.
The final Save omitted the response wait already used for the first Save.

Arm an ID-correlated `workspace.file.update` wait for the renamed path and
session before clicking Save. Require success before reading exact disk bytes.
Keep the previous five-second observation budget, source-absence checks, dirty
content, tab identity, and reopen assertions. Retain first-failure traces.
No production file-write or rename behavior changes.

A disposable per-worker Git wrapper pauses real Git after unlink for 500 ms.
The original test reproduces ENOENT during that gap. With the same pause, the
receipt-based check passes and all original assertions complete. The wrapper is
restored before cleanup and no instrumentation enters the committed fixture.
This controlled proof does not establish the historical CI syscall timing,
which is unavailable without the failed attempt's trace. Normal repeated
rename/save cases, lint, typecheck, hooks, and fresh CI remain required.

Normal rename and save-refresh scenarios pass three repetitions: six cases,
retries disabled. Formatting, zero-warning scoped lint, full typecheck, and
documentation/specification checks pass. Independent review finds no actionable
issue. Normal hooks, delivery, and fresh CI remain required.
