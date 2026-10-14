---
created: 2026-10-08
status: done
requirements:
  - REQ-TASKS-DOCUMENTS-003
system_design:
  - ../../specs/tasks/system-design/plan-write-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Preserve plan typing during save acknowledgements

## Overview

Preserve typing made after a plan save starts while allowing the acknowledged
snapshot to become the persisted baseline. Deliver one sequential work order
inside the local draft lifecycle, with real hook/store regressions followed by
the minimum production correction and a real panel/editor integration check.
ROOT reviewed this design package and later explicitly released implementation
to this primary. The single scoped work order is now complete.

## Scope

### In scope

- The successful-save draft contract in `REQ-TASKS-DOCUMENTS-003`, extending
  the existing Task Documents owner and persistence lifecycle design.
- Local own-attempt recognition before dispatch, acknowledgement consumption,
  persisted baseline versus live draft, and editor-key continuity.
- Compatibility controls for unchanged success, manual/autosave callbacks,
  overlapping attempts, task switches, genuine external updates, and existing
  size suppression/generic retry behavior.
- Focused real hook/store tests and a real panel/TipTap integration test.

### Out of scope

- Backend, public API, store actions, event/version policy, saving-state
  refcounting, transport reordering, navigation, layout, controls, and copy.
- General concurrent-writer reconciliation, text merging, collaboration,
  persistence of unsaved drafts across navigation, and save queues.
- Installs, product tests, browser/build/full suites, staging, commits, or
  publication during the design turn. No delegation or new persistent tasks,
  sessions, tabs, or model switches.

## Evidence and assumptions

ROOT's accepted read-only archive is
`/tmp/kandev-root-plan-save-discovery-20261008`. Its regular mode-0400
`candidate.test.tsx` has SHA256
`8cdb2e85768bd20a8f45f8fc86eee35912b9347b1103138b236238679dd69b3a`.
`qualified-proof.json` records actual Vitest exit 1, wrapper exit 0, native
handle 71013/start chunk `337e1a`, actually joined terminal chunk `b1b138`,
one causal failure, and two passing controls. The failed case submits A,
types B while the real hook is saving, then receives A in the real store and
loses B. Controls verify unchanged submitted text becomes clean and a clean
editor adopts an authoritative agent update. The earlier Python authoring
syntax failure started no command and is not RED. The archive is evidence,
not a fixture to copy, import, replay, edit, chmod, or clean.

The proof base `4d24f78` to design base
`f61fd4e5fda79c0bb28f94e6af1e988898be9561` changes only backend unstage,
Windows CI, and associated docs. No frontend path differs. Current source
confirms the defect: `useTaskPlan.savePlan` publishes A, then
`usePlanDraft`'s content effect replaces differing B and increments `editorKey`.
`TaskPlanPanel` allows editing while saving (`readOnly` is only `isLoading`).

The caller settled the repair scope and desired external-update/failure
semantics. There are no unresolved product decisions. The task system owns
the contract because plans are backward-compatible task documents; the UI
system does not own a separate requirement for this outcome. Existing
size-limit documents own rejection semantics and remain referenced controls.
No new ADR or incident-specific requirement document is needed.

## Technical approach

Implement in `apps/web/hooks/domains/session/use-plan-draft.ts`. Retain the
public hook consumers and save transport order. Register local ownership
before calling `savePlan`, scope it to the current task view and own attempt,
and recognize/consume the corresponding published snapshot without assigning
it over newer draft text or incrementing the editor key. Distinguish an
observed persisted baseline from a truthy but unpublished stale save result.

Keep ownership recognition separate from rejected-content suppression.
Completion callbacks may retire only the attempt/view they own, including
identical-text overlapping submissions and A-to-B-to-A navigation. Expire
recognition rather than permanently ignoring every previously submitted
string. Keep genuine external replacement, deletion, and the external-sync
autosave guard. Do not use `isSaving` as source provenance or request ordering.
The owning [design](../../specs/tasks/system-design/plan-write-lifecycle.md#plan-draft-acknowledgement)
defines the publication-before-promise ordering and bounded correlation model.

`TaskPlanPanel`, `PlanPanelHeader`, `TipTapPlanEditor`, `StateProvider`,
`createAppStore`, and `useTaskPlan` are real integration consumers, not new
production ownership. Their existing save/error/latest-attempt behavior is
preserved. A production edit outside the local draft hook requires a ROOT
scope checkpoint before proceeding.

## Tests

Author `apps/web/hooks/domains/session/use-plan-draft.save-ack.test.tsx`
independently from current source. Use real StateProvider/store and real
hooks; mock only plan transport with deferred responses. Do not rerender
synthetic saved props or replace internal hooks/provider. Name the primary
regression `keeps newer typing after its own autosave acknowledgement`.
Prove RED from loss of B after A's real store publication, not from an import,
fixture, missing method, timeout, or tautological ownership predicate.

| Acceptance | Focused evidence |
| --- | --- |
| 003.1, 003.2 | Own deferred autosave A, edit B while saving, store A, draft B, unchanged editor key, dirty true, exact next request B; existing-plan update and first-plan create |
| 003.3 | Unchanged submitted content becomes clean without duplicate requests beyond several debounce intervals; a subsequent edit saves normally |
| 003.4 | Genuine external update replaces clean and dirty drafts; deletion empties; external content matching a completed or failed old submission is still adopted |
| 003.5 | Explicit save shares protection; same-task overlapping different/identical submissions in both completion orders preserve latest publication and later size suppression; unpublished stale success never cleans draft |
| 003.6 | A-to-B, A-to-null, no-plan/equal-content switches, A-to-B-to-A, and delayed success/failure callbacks; real store background publication stays task-owned and outgoing callbacks cannot clear new suppression |
| 003.7 | Actual size rejection preserves draft and baseline, suppresses unchanged autosave across saving transitions, permits edited/explicit retries; generic failure re-arms |

Author `apps/web/components/task/task-plan-panel.save-ack.test.tsx` for the
real panel, dynamic adapter, TipTap editor, hooks, and store. Produce edits
through editor transactions/onChange and deferred transport, then verify DOM
identity, text, selection/focus, unsaved state, and B's next save. Use narrow
existing environment shims and unrelated chrome/service mocks only. This
test provides consumer integration beyond hook state assertions.

Run the existing draft, use-task-plan, header, session-switch, and TipTap
editor suites with the new suites. Preserve their assertions and timing.
The work order owns exact RED/GREEN and affected checks; record actual test
counts and original terminal verdicts there after implementation.

## End-to-end and mobile assessment

The consumer path is transport -> real store -> real hooks -> real panel ->
real TipTap editor -> onChange -> next transport request. This is the focused
component integration boundary for this state-only repair; it is not a claim
of full browser/WebSocket evidence. Both pointer modes share draft behavior.
Where practical, exercise the real component case with fine-pointer and phone
breakpoint inputs using existing test setup. The mobile-parity state/data
exception permits targeted unit/component proof without new mobile Playwright
coverage. No markup/layout/touch/scroll/navigation/breakpoint changes require
an ASCII UI preview. No browser, build, or full suite is scheduled.

## Documentation audit

The existing public task-plan guide promises direct editing and autosave after
1.5 seconds. This restores that behavior; no public steps, screenshots,
terminology, API, or config contract changes. Internal docs are updated in
the owning requirement/design and this delivery package. Existing attachment
and size-limit delivery packages are independent and remain unchanged.

## Work orders

- [x] [Task 01: Preserve own-save draft continuity](task-01-save-ack-continuity.md)

Exactly one sequential work order, with no dependencies. ROOT reviewed all
four design artifacts, released author-only implementation, then granted an
exclusive local-heavy implementation/checks/publication lease on 2026-10-08.
The work order is `done`; publication and merge use the live task-plan gates.

## Verification results

Implementation results on 2026-10-08: the permanent real-store regression
failed causally before the hook correction, then all 64 tests across the seven
planned suites passed. Provider fixture and duplicated-literal repairs reran
only their affected new suites. Changed-file lint, web typecheck, both i18n
checks, catalog validation and spec lint passed. Actual coverage of all seven
changed paths returned `status=covered`, `errors=[]`. The work order records
exact original handles, terminal chunks and counts. Only the local draft hook
changed production behavior; real desktop/phone TipTap continuity is covered.

The checkpoints below are historical design/author evidence, before ROOT's
later explicit implementation and heavy releases.

Design-only results on 2026-10-08:

- `python3 scripts/list-docs.py validate`: exit 0; 363 decisions and 1437
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specifications passed.
- Repository `validateCoverage` against actual four changed paths: exit 0,
  `status=exempt`, `errors=[]` (documentation-only diff).
- Separate prospective reference preflight including the planned production
  hook path: exit 0, `status=covered`, `errors=[]`; the work order's requirement,
  seven acceptance criteria, design, and manifest references are accepted.
  This is reference validation, not executed production-path coverage.
- Local Markdown target existence check: all four files and targets passed.
- `git diff --check`: exit 0. Status contains exactly the owning pair and new
  manifest/work order, unstaged; cached diff is empty.

The first reference-preflight invocation did not start because `node` was not
on PATH (shell exit 127). Read-only discovery found the already installed
`/home/jcfs/.local/share/mise/installs/node/24/bin/node`; using that executable
completed the cheap reference checks. No toolchain install or product command
was run. `apps/node_modules` is absent; later execution requires ROOT's lease
and the conditional single frozen install. Implementation RED/GREEN, lint,
typecheck, and i18n remain pending.

Author-only implementation checkpoint: ROOT reviewed the four artifacts and
accepted the original hashes/base in
`/tmp/kandev-root-child80-reviewed-design-20261008.json`. Its later release
admitted independent permanent regression authoring, with no heavy lease.
The one work order is now `in_progress`. Both planned fixtures are authored;
production remains unchanged. No test, install, lint/typecheck/i18n, build,
hook, commit, or push was run. Cheap whitespace/status check returned exit 0
in native terminal chunk `cdc1c2`; no running handles or groups are owned.
AUTHOR_READY_FOR_HEAVY: await the separate ROOT grant, then run the exact
work-order RED after the conditional one pinned frozen install.

## Resources and delivery

The live task plan for task `f4d20149-9c91-4fda-b729-4fa127d29dcc`, session
`95b77a40-2dae-4682-af51-3679ea0b592b`, preserves the system marker, task/title
ownership, caller constraints, proof receipt, resource state, and crash next
action. No local-heavy resources are owned during design. ROOT alone grants
heavy execution and a later serial merge. Implementation commands use pinned
pnpm 9.15.9, one worker, and Node 4 GiB; retain and actually join original
handles. Unknown/resource/timeout/transport/out-of-scope outcomes checkpoint
ROOT without autonomous retry. Publication and hosted/merge gates remain
separately constrained by the live task plan.

## Risks

- Store publication precedes the wrapper continuation. Recognition registered
  too late can pass a synthetic fixture while still replacing live typing.
- Comparing only content can let an old same-text success clear newer failure
  suppression. Use attempt and task-view ownership with real overlap controls.
- Treating all changes while saving as own acknowledgements hides genuine
  external content. Keep bounded receipt recognition and expiry.
- Happy-dom TipTap fixtures can fail on browser-only setup. Use established
  editor-test shims; checkpoint infrastructure gaps instead of replacing the
  actual editor or weakening assertions/timeouts.
