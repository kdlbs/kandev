---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-EDIT-FOCUS-001
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
legacy_specs: []
---

# Implementation Plan: Sidebar task Rename ownership

## Overview

Restore AC-UI-EDIT-FOCUS-001.7 for the separate `useTaskRowActions` producer.
The supplied real-provider/API/dialog reproduction proves that unconditional
request finalization removes a later different-task or reopened same-task edit.
The user explicitly authorizes this bounded package and TDD in the same turn.

## Scope and approach

Capture the original edit object and clear only that still-current object through
a functional state update. Preserve target/title capture, normalization, immediate
dialog close, error logging, explicit Save/Cancel and raw input/caret.
Own only `task-session-sidebar.tsx` rename finalization, faithful regressions,
caller verification and this package. No dependency beyond current main and the
existing inline-edit requirement/design pair. Keep the mobile guard unchanged.
Exclude external-link dialogs (sibling ownership), Query/cache changes, backend,
API/schema, new feedback, cancellation, global queues, shared abstractions,
navigation, menu primitives and persistent drafts.

## ASCII UI preview

UI-01: Shipped sidebar row menu or task command palette > Rename.
Desktop/tablet retain the short centered form below; spacing is illustrative.

```text
A Save -> dialog closes immediately; PATCH A pending
Later A or B: [Rename task]
              [ exact newer draft ]
              [Cancel]       [Save]
Old A success/failure:
  before -> later dialog disappears
  after  -> same input/draft/caret remains; Cancel/Save works
```

UI-02: Phone TaskCommands retains the shipped responsive dialog: title, input,
primary Save, then Cancel in a vertical footer. No new navigation/scroll owner.

```text
[Rename task]
[ exact newer draft ]
[       Save       ]
[      Cancel      ]
```

AC-UI-EDIT-FOCUS-001.7 governs completion ownership. TaskCommands uses this
producer on desktop and phone; do not label it desktop-exclusive. The separate
phone Tasks picker uses already-correct `useMobileTaskRename`. Its shipped picker
and TaskRenameDialog are the mobile exemplars. No surface, touch, scrolling,
breakpoint or layout changes are needed. The mobile-parity state/data exception
applies; retain the existing phone/tablet browser controls and trace actual callers.

## Work orders

- [x] [Task 01: Preserve row edit](task-01-preserve-row-edit.md)

## Verification strategy

Real providers, all composed row hooks, action/API and dialog with only fetch
held provide causal RED then GREEN for success/failure, same/different target,
reverse settlements, ordinary dismissal, current-owner and explicit controls.
Shipped sidebar and command palette run against a disposable real backend with
held transport, explicit worktree executor and unchanged seed repository.
Browser emulation establishes integration, not physical-device/incident proof.
Exact commands and results belong to the work order. Receipts live outside the
checkout. Public documentation assessment covers existing Rename instructions.

## Risks and delivery

Task ID cannot identify a reopened edit. Feedback and admitted writes retain
their existing ownership. Refresh main/inventory/target patches before delivery.
After checks, use normal active hooks and produce a clean local commit, send the
queued parent validation checkpoint, and stop before push/PR. Separate same-session
delivery authorization is required.

## Original implementation results

- Frozen install passed before permanent tests/production.
- Causal component RED: 6 ownership failures after settlement, 4 ordinary/current
  owner controls passed. Final focused run: 25 passed in 3 files (6.21s).
- Browser RED: 3 intended sidebar ownership failures. The command case hit an
  accessible-name fixture error (the option includes the task title); it is not
  causal bug evidence. Corrected selector follows the shipped palette text.
- Initial post-fix browser run retained drafts but exposed synthetic caret setup
  racing autofocus on 2 success cases; failure case passed. Final helper waits
  for focus/finite animations and establishes caret with real keyboard events
  before releasing transport, then asserts that same caret after completion.
- Fresh production build, real disposable backend, explicit worktree executor,
  one worker/shard and retries zero. Final desktop screenshot run: 4 passed
  (27.4s), including sidebar same/different task, error settlement, and TaskCommands.
  Final phone/tablet TaskCommands screenshot run: 2 passed (12.8s), using touch
  activation. Existing distinct mobile picker compatibility: 3 passed within the
  preceding 5-case phone/tablet run (32.8s); its guard is byte-identical to base.
- Exact raw draft/caret, captured normalized PATCH, immediate dismissal, later
  Save/Cancel, unchanged non-target task, unchanged route, persisted titles and
  seed repository HEAD/status checked. Source snapshots matched across runtime
  runs. Desktop/phone/tablet retained-draft screenshots inspected against UI-01/02.
- Scoped ESLint passed with zero warnings. Node 24 full typecheck passed with
  CI's 8 GiB heap cap; rich-output catalog check passed. i18n check passed with
  complete locale catalogs. No new copy or translations.
- Specification linter tests: 36 passed; documentation catalog tests: 9 passed.
  Final catalog/full-spec/diff/coverage results and normal hook receipt are
  recorded in the work order and external readiness receipt.
- Public docs assessment: existing Rename entry points, instructions, labels,
  API, settings and screenshot contracts are unchanged. This restores the
  existing edit-lifetime guarantee; internal design/plan updates suffice.
- Browser emulation plus real backend transport proves shipped UI integration;
  physical device/OS keyboard and operator-runtime incident behavior remain
  unverified. No external credentials/services or operator data used.
- All receipts are outside checkout at
  `/home/jcfs/.kandev/tasks/preserve-newer-sideb_kd5ueztx/receipts/attempt23`.
  Normal active hooks, clean commit and terminal handles precede the queued
  parent validation checkpoint. That original checkpoint did not authorize push/PR. Parent review subsequently
  granted same-session delivery; fresh hosted gates remain required.

## Hosted fixture safety remediation

The first hosted run at the original implementation head passed required checks,
but its retry audit found one new sidebar case failing the worker-seed HEAD
guard (clean status; HEAD changed from f4640337 to ee5e472e). Rename UI and
persisted-title checks reached the final state. Local retries-zero reproduction
passed all four cases; this does not disposition the source safety observation.
Hosted artifacts retain only the SHA pair, context/screenshots and retry trace,
not the Git objects/subjects or full writer process history. Prior file-tree
content proves worker reuse but cannot identify the writer.

Bound this test family to an owned temporary source repository/offline origin,
workspace and workflow, with explicit worktree executor and existing agent.
Retained own-source and worker-seed HEAD/status guards. Cleanup runs in finally,
deletes only owned resources, and preserves original/cleanup failures. Added a
real-backend failure-path cleanup control. Keep the application correction, mobile
guard, sibling files and six other recovered retry surfaces untouched.

Causal RED is the native source-safety assertion above. Verification uses the
existing scoped browser commands (desktop now includes the cleanup control),
phone/tablet compatibility, scoped lint/static/docs gates and normal hooks.
A new test-only remediation head requires fresh CI and substantive reviews.
Local remediation results: 5 desktop cases passed (26.2s), including the real
failure-path cleanup control; 5 phone/tablet cases passed (29.7s), including the
3 unchanged distinct mobile-picker cases. All runs use one worker and retries
zero. The first mobile command selected a nonexistent project and stopped before
tests; its setup error remains separate from the corrected run. Owned source and
worker seed guards passed; source cleanup was exercised on success and failure.
The existing 25 focused component/caller tests, scoped zero-warning lint,
Node 24 typecheck/catalog with 8 GiB cap, i18n, full specification/catalog checks
and full-base documentation coverage are rechecked for this test-only change.
Hosted resource snapshots add only CPU/memory counters, not writer lineage.
The prior seed writer remains unattributed. The fixture boundary is the bounded
safety disposition; a local nonreproduction is not root-cause evidence.
Fresh exact-head CI/review and normal protected merge evidence are recorded
externally after the test-only commit. No other retry surface is changed.
