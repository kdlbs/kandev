---
id: "01-preserve-current-link"
title: "Preserve current GitLab link form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-ACTIONS-MENU-004
acceptance_criteria:
  - AC-TASKS-TASK-ACTIONS-MENU-004.8
system_design:
  - ../../specs/tasks/system-design/task-actions-menu.md
---

# Task 01: Preserve current GitLab link form

## Summary

Retire only old form authority while letting its original link finish. Cover
conditional/retained forms and the actual desktop/phone menu path.

## In scope

MR dialog state and completion ownership, real-flow regression tests, disposable
browser fixtures, and synchronized verification results.

## Out of scope

Kanban rollback, external-issue forms, creation/commit dialogs, API/backend,
provider permission/policy, store migrations and generic form abstractions.

## Acceptance

1. Old completion publishes only its original association and truthful feedback;
   later draft, selection, focus/caret and busy state survive.
2. Current success closes; failure retains draft for retry; defaults/hydration
   and ordinary refresh behavior pass existing coverage.
3. Actual desktop and touch Link paths pass with all owned resources and handles
   closed, normal hooks and a clean commit before parent validation.

## ASCII UI preview

UI-01: Link menu -> GitLab Merge Request, replacement form after old ACK.
Desktop uses the sidebar/task menu; phone uses the existing task-picker sheet
and touch menu. The existing dialog stacks its footer actions on phones.

```text
Before (observed)       After (desktop and phone)
[replacement form]     [Link GitLab merge request]
old ACK -> disappears [Merge request URL: raw newer URL]
                      [Task repository: current choice]
                      [Cancel] [Link merge request]

UI-01 phone: same fields; existing stacked footer
[Link GitLab merge request]
[Merge request URL: raw newer URL]
[Task repository: current choice, when multiple]
[Link merge request]
[Cancel]
```

URL, repository and responsive footer order use existing primitives and translations.
No layout or sizing changes. Focus/caret stay in the new input on old settlement;
current failure keeps the form and error toast. The drawing illustrates
AC-TASKS-TASK-ACTIONS-MENU-004.8, not pixel dimensions. Mobile exemplar:
`mobile/session-task-switcher-sheet.tsx` and its retained dialog host. The sheet
owns task navigation scrolling; the existing short Radix dialog owns the form.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/task-mr-link-ownership.test.tsx components/gitlab/task-mr-link-dialog.test.tsx)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 NODE_OPTIONS=--max-old-space-size=8192 pnpm e2e:run --host --shards 1 --project chromium -- tests/gitlab/mr-link-ownership.spec.ts --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 NODE_OPTIONS=--max-old-space-size=8192 pnpm e2e:run --host --shards 1 --project mobile-chrome -- tests/gitlab/mobile-mr-link-ownership.spec.ts --retries=0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/gitlab/task-mr-link-dialog.tsx components/task/task-mr-link-ownership.test.tsx e2e/tests/gitlab/*mr-link-ownership*.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm run e2e:sleep-ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository PR-documentation coverage preflight after staging. Refresh
open-PR inventory and the identified target patches before the checkpoint.

## Files likely touched

- `apps/web/components/gitlab/task-mr-link-dialog.tsx`
- `apps/web/components/task/task-mr-link-ownership.test.tsx`
- `apps/web/e2e/tests/gitlab/mr-link-ownership-helpers.ts`
- `apps/web/e2e/tests/gitlab/mr-link-ownership.spec.ts`
- `apps/web/e2e/tests/gitlab/mobile-mr-link-ownership.spec.ts`

## Dependencies

None. Frozen workspace install before pnpm commands.

## Risks

Same as [plan](plan.md#risks). Cleanup must preserve the original failure and
all cleanup failures rather than masking either.

## Parallelism

`sequential`; no subagents.

## Inputs

AC004.8 and the native GitLab lifetime section of the linked design. Existing
external-link ownership tests supply the real coordinator/provider pattern;
read only, with no changes to their implementation.

## Results

Completed locally, before parent validation and publication.

- Frozen workspace install passed (Node 24.18.0).
- Corrected actual-flow RED: 9 ownership failures and 11 passing controls,
  including all four existing hydration/refresh tests. Initial missing-file and
  selector scaffolding logs are retained separately, not counted as RED.
- Corrected desktop RED: 2 intended post-ACK missing-input failures after
  original POST payload and persisted association assertions passed. Its first
  attempt stopped at a test response-shape error; that setup log is retained.
- Phone RED: the same 2 post-ACK failures through `.tap()` on the shipped picker
  and Link menu. Every failure run completed its finally cleanup.
- Final focused Vitest: 23 passed (19 actual coordinator/provider/API/store
  integration cases and 4 existing default/hydration/refresh controls).
- Fresh production E2E build passed; desktop 2/2 and phone 2/2 passed, one worker,
  one shard, retries 0 and strict WS accounting. The final artifact-only test
  update saved screenshots and explicit resource-closure JSON; desktop and
  phone were rerun against those unchanged production assets with `--no-build`.
- Full web typecheck (8 GiB cap) and rich-output generated catalog check passed.
  The first typecheck found invalid test-selector options, corrected before the
  final run. Initial lint found duplicated test literals; final lint has zero
  warnings. No product code changed for those test tooling corrections.
- Full i18n checks passed; staged-file ratchets and normal hooks are part of
  the clean-commit receipt preserved outside the checkout.
- Catalog validation: 397 decisions and 1710 specifications. Specification
  linter: passed. Specification-linter tests: 36 passed.
- PR-documentation coverage preflight uses `.github/scripts/pr-docs.cjs` with
  the exact staged paths and on-disk referenced artifacts. The full receipt,
  normal pre-commit/commit-msg results and complete binary diff are preserved
  outside the checkout for the parent validation checkpoint.
- Duplicate refresh: #1512 still only changes sidebar Query lookup at
  a8655ad92faa050a68140a904500fc99ebc0a04b; #4517 and #4444 have no matching
  target files. All paginated file lists and current open inventory retained.
- Public docs assessed across `docs/public`, README and screenshot catalog:
  no update needed. This repairs the existing Link workflow; labels, navigation,
  permission/provider/API contracts and operator instructions do not change.
- Internal docs updated: one Tasks requirement/design pair and this one plan
  with one work order. No ADR or duplicate Integration/UI contract.
- Fixture finally blocks delete owned tasks/worktrees/workspaces, compare
  original worker seed and owned source HEAD plus complete raw porcelain, and
  remove owned roots. No executor profile is created. Primary and cleanup
  failures remain separately reported. Final per-case closure JSON confirms
  no cleanup failures. No operator runtime, database, user seed or credentials
  were used or mutated.

Evidence limits: component integration and Chromium browser emulation against a
real disposable backend with mock GitLab. No physical OS/device, real-provider,
operator-incident, server-data-loss or zero-flake claim. The parent must review
the full diff and independently rerun focused checks, then issue a separate
same-session delivery grant. No push or PR is authorized at this checkpoint.

## Review remediation: passive repository hydration

The substantive exact-head CodeRabbit review found that passive default
hydration could retire a pending admission and enable another submission. Two
actual retained-form/provider/API/store regressions failed at the pending-state
assertion before correction. A local selection epoch now advances only on an
explicit repository selection, keeping passive hydration owned by its admitted
request. The original trimmed payload and absent repository remain captured;
current success closes and failure retries with the hydrated default.

The focused suite now contains 25 cases (21 actual-flow ownership cases and
4 existing hydration/refresh controls). Initial 23-case evidence above is
historical. Final remediation verification and exact-head CI/review evidence
are recorded in the delivery receipts outside the checkout. No provider/API,
public copy or navigation change; no public docs update is needed.

Remediation local results: 25 focused tests passed; strict changed-file ESLint,
full web typecheck (Node 24, 8 GiB), rich-output catalog, documentation catalog
and full specification lint passed. Fresh rebuilt desktop and phone each passed
three scenarios with retries disabled (the two registered tests plus one
capture scenario). Normal hooks and post-commit recaptures follow in the
same-session delivery receipt; no tests or workflows were suppressed.
