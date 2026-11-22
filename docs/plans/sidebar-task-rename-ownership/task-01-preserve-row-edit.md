---
id: "01-preserve-row-edit"
title: "Preserve new row Rename edits through old completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EDIT-FOCUS-001
acceptance_criteria:
  - AC-UI-EDIT-FOCUS-001.7
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
---

# Task 01: Preserve row edit

## Outcome and ownership

An earlier row Rename settlement preserves every later edit, including the same
task. Own `apps/web/components/task/task-session-sidebar.tsx` finalization,
`task-session-sidebar-rename-ownership.test.tsx`, focused browser tests under
`apps/web/e2e/tests/task/`, minimal existing design mapping and this package.
No delegation. Preserve others' edits and the mobile guard byte-for-byte.
Dependencies: existing AC-UI-EDIT-FOCUS-001.7 and its current system design.
Exclude sibling link dialogs, cache/mutation redesign and all scope exclusions
in the [plan](plan.md).

## Acceptance

1. Causal real-producer/API/dialog RED fails after old settlement; GREEN preserves
   raw draft/caret for same/different tasks, success/failure and reverse order.
2. Original normalized PATCH and immediate dismissal remain; later Save/Cancel,
   ordinary settlement, current-owner completion and existing error log work.
3. Shipped sidebar/TaskCommands and phone/tablet compatibility are verified with
   honest limits, all owned handles terminal, normal hooks and clean commit.

## ASCII UI preview

UI-01, from the [plan](plan.md): existing row/menu or command palette > Rename.

```text
A Save -> closes; PATCH A pending
Later A/B: [Rename task] [ exact newer draft ] [Cancel] [Save]
Old A settles -> same later editor remains available
```

UI-02: Phone retains the existing title/input and vertical Save then Cancel
footer, as shown in the plan. Completion preserves this same surface.

Phone TaskCommands shares this producer. The phone Tasks picker retains its
distinct corrected mobile hook and existing dialog. No layout/touch changes.

## Verification

Run from repo root with Node 24 and Go 1.26 on PATH; use
`NODE_OPTIONS=--max-old-space-size=8192` only for full typecheck (CI's 8 GiB cap),
`GOMAXPROCS=2 GOFLAGS=-p=2` for builds, one E2E worker/shard, retries zero.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/task-session-sidebar-rename-ownership.test.tsx components/task/mobile/use-mobile-task-rename-ownership.test.tsx components/task-commands.test.tsx)
scripts/run-quiet e2e --summary -- bash -c 'cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/task/sidebar-task-rename-ownership.spec.ts --retries=0'
scripts/run-quiet e2e --summary -- bash -c 'cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-row-task-rename-ownership.spec.ts e2e/tests/task/mobile-task-rename-ownership.spec.ts --retries=0'
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/task-session-sidebar.tsx components/task/task-session-sidebar-rename-ownership.test.tsx e2e/tests/task/sidebar-task-rename-ownership.spec.ts e2e/tests/task/sidebar-task-rename-ownership-helpers.ts e2e/tests/task/sidebar-task-rename-fixture.ts e2e/tests/task/mobile-row-task-rename-ownership.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-spec-files.test.py
python3 scripts/list-docs.test.py
git diff --check 07ba3a68b31f837bac0f4df5a6b3fa6644f20051
git status --short -- docs/plans/sidebar-task-rename-ownership
```

Also run full-base documentation coverage preflight with its repository CLI and
inspect the complete base diff; record exact command below. Source/seed snapshots
must remain unchanged by runtime use. No operator runtime or credentials.

## Original implementation results

See the [original plan results](plan.md#original-implementation-results) for causal RED/GREEN, fixture corrections,
caller/viewport proof and evidence limits. All three final focused test files
passed (25 tests); final browser captures passed 4 desktop and 2 phone/tablet
cases. Existing mobile picker compatibility passed 3 cases. Runtime snapshots
matched and explicit worktree seed HEAD/status remained unchanged. Node 24
8 GiB typecheck/catalog and i18n passed; scoped lint has zero warnings; 36
specification-linter and 9 catalog tests passed. No public docs update is needed:
Rename entry/instructions are unchanged and the existing UI contract is restored.

Full-base documentation coverage command:
`node /home/jcfs/.kandev/tasks/preserve-newer-sideb_kd5ueztx/receipts/attempt23/docs-coverage.cjs`.
It invokes the repository's `validateCoverage` against the full base diff plus
new files and the one work order/existing requirement/design pair. Final result:
covered. Catalog validation (396 decisions, 1701 specifications),
full specification lint and full-base diff whitespace gates passed. Commit hook
results are retained in the external readiness receipt. The original checkpoint stopped after clean local commit and queued parent
notice, before push/PR. Parent review subsequently granted same-session delivery.

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
