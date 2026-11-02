---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-EDIT-FOCUS-001
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
legacy_specs: []
---

# Implementation Plan: Mobile task Rename ownership

## Overview

Preserve a later Rename edit when an earlier request settles. The parent proved
that unconditional finalization closes both a different task and a reopened
same-task draft. Prepare this package, reproduce with held transport, then apply
the bounded fix. The user explicitly authorizes package and TDD in one turn.

## Scope

Own `useMobileTaskRename` completion admission and faithful regressions. Extend
only the existing inline-edit continuity pair: edit lifetime is reusable UI
state, while Tasks retains mutations. This extends its existing captured-target
boundary; no ADR is needed for the local identity comparison.

Exclude backend/API/schema, cancellation of admitted writes, global queues,
persistent drafts, shared menu/modal primitives, navigation and cache redesign,
and sibling notification/settings work. Preserve others' changes.

## Technical approach

Compare the current edit object with the submission's captured object inside a
functional state updater. Each open already creates a fresh object, so same-task
reentry is distinct. Keep success/error settlement and failure feedback intact.

## ASCII UI preview

UI-01: Existing phone Tasks picker > row Task actions > Rename. Existing dialog
composition and scroll/safe-area/input sizing are unchanged. Exact spacing is
illustrative. AC-UI-EDIT-FOCUS-001.7 changes completion admission only.

```text
Earlier A Save -> dialog closes immediately; PATCH A remains pending
Later A or B: [Rename task]
              [ exact newer draft ]
              [Cancel]       [Save]
Old A settles: before -> later dialog disappears
               after -> same draft remains, Cancel/Save still works
```

Desktop/tablet title/sidebar edits retain their shipped entry points and handlers;
trace callers rather than infer a viewport distinction from the hook's name.
Nearest exemplar is the shipped Tasks picker and TaskRenameDialog. No surface,
geometry, pointer handler, navigation, or scrolling changes are proposed.

## Tests

`components/task/mobile/use-mobile-task-rename-ownership.test.tsx` uses actual
StateProvider/ToastProvider/action/API/dialog and held fetch: different/same task,
success/failure, ordinary settlement, out-of-order writes and explicit controls.
The existing hook/dialog tests retain current-owner and normalization/focus
controls. Covers AC-UI-EDIT-FOCUS-001.4, .5, .7.

## E2E tests

`e2e/tests/task/mobile-task-rename-ownership.spec.ts`, `mobile-chrome`: actual
mounted SessionTaskSwitcherSheet/dialog and touch entry, held original task PATCH,
newer draft retained for different/same task, plus same-task reentry across a
900px tablet resize, then Save/Cancel; disposable backend
and explicit worktree executor. Check seed/source unchanged. Existing
`inline-edit-focus.spec.ts` provides desktop/sidebar compatibility. Phone emulation
is not a physical device or incident proof. If fixture limitations block the
native touch flow, retain component proof and disclose the exact blocker; the
mobile-parity state/data exception applies to this unchanged presentation.

## Work orders

- [x] [Task 01: Preserve new edit](task-01-preserve-new-edit.md)

## Verification results

- Frozen `pnpm install --frozen-lockfile` from `apps/`: passed.
- Causal component RED: 6 intended ownership failures, 2 ordinary controls passed;
  real providers/dialog/action/API, only fetch held. Final focused run: 11 passed
  in 3 files, including current-owner controls and retained caret/normalization.
- Actual mounted picker/browser RED: 2 intended missing-newer-draft failures
  after old response, including corrected explicit agent/worktree fixture.
  An earlier fixture run had unrelated missing-agent session notices; the
  corrected run reproduced the same ownership failures without those notices.
- Fresh production build + phone touch GREEN: 2 passed. Final browser run:
  3 passed (20.3s), covering different/same task and same-task edit after a
  900px tablet resize. Explicit Save/Cancel and persisted titles checked.
  Seed repository HEAD/status remained unchanged. The earlier fresh-build
  two-scenario run also verified all changed/untracked source bytes unchanged.
- Existing desktop top-bar, Office and sidebar dialog focus/edit compatibility:
  3 passed (9.6s), using the same fresh build. These are ordinary edit controls,
  not a claim to repair the separate desktop sidebar completion handler.
- Scoped ESLint: passed with zero warnings. Initial duplicate test-string
  warnings were corrected. Node 24 typecheck passed after removing unsupported
  Testing Library `exact` options from the new component test. i18n check passed.
- Documentation catalog validation, full specification lint, 36 spec-linter
  tests and 9 catalog tests: passed. Full-base documentation coverage preflight:
  covered by this one work order and the existing requirement/design pair.
- Public docs assessment: existing task Rename entry/instructions are unchanged;
  this restores an edit-lifetime guarantee and introduces no new workflow, copy,
  API, configuration or screenshot contract. Internal specs/plans updated only.
- Screenshot inspection confirms the retained newer draft and unchanged controls
  on phone and tablet. Browser device emulation and mocked agent/disposable real
  backend establish shipped transport/UI integration, not a physical phone,
  native OS keyboard, or operator-runtime incident claim.
- Normal active precommit/commit-msg hooks and clean committed HEAD are mandatory
  for the parent checkpoint. Exact SHA, hook results, terminal process ledger,
  full-base diff and logs are recorded outside checkout in the readiness receipt.
  No push or PR is authorized at this checkpoint.

Before edits, remote main matched edafc7900de0328a9997401f11a0d76cb8c548d3;
69 open PRs, actual paginated target paths empty for 1512/4013/4444. This is a
bounded overlap check, not an unrestricted PR correctness review.

## Risks

Same-task identity must use edit provenance, not task ID. Do not gate legitimate
failure feedback with dialog ownership. Browser fixtures must stay isolated.
Delivery stops after normal hooks and a clean commit for parent review; no push
or PR without separate same-session authorization.
