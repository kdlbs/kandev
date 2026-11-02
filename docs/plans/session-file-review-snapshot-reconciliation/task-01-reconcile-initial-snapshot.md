---
id: "01-reconcile-initial-snapshot"
title: "Reconcile initial review snapshots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.12
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.13
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.14
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 01: Reconcile Initial Review Snapshots

## Summary

Retain local mark/unmark/reset intent against an older initial snapshot.
Write permanent causal RED first, then the minimum hook-only reconciliation.

## Scope and owned files

- `apps/web/hooks/use-session-file-reviews.ts`
- `apps/web/hooks/use-session-file-reviews.test.tsx`
- Owning task-navigation-responsiveness requirement/design and this package.

No consumer production change, persistence/API/schema change, arbitrary
write/write concurrency policy, cache lifetime redesign or framework migration.

## Acceptance

- Pending and acknowledged mark/unmark/reset survive older initial GET; one-file
  changes retain unrelated rows and reset suppresses all older rows.
- Single mark failure retains current rollback in either settlement order;
  unchanged GET and shared/captured reader lifetime behavior remain valid.
- Real ReviewDialog checkbox action stays accepted when old GET completes;
  only deferred wire transport is mocked, with actual providers/client/hash.

## Verification

Run from repository root with Node 24 on PATH:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-session-file-reviews.test.tsx --maxWorkers=1 --retry=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-session-file-reviews.ts hooks/use-session-file-reviews.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the unchanged `.github/scripts/pr-docs.cjs` `validateCoverage` against actual
changed/untracked paths with the complete requirement/design/plan/work-order
closure, and require `covered` after source changes. Save exact invocation and
JSON externally. Inspect full staged/untracked diff, normal active hooks and
clean committed HEAD. All live command handles must be joined to terminal.

## Mobile and public docs

The [manifest assessment](plan.md#mobile-e2e-and-public-documentation) applies.
The state/data exception uses existing native/shared consumer paths; no new UI
entry point, layout preview or browser test is invented.

## Dependencies and parallelism

None. Sequential in this session; no nested agents/tasks/sessions.

## Inputs

- Current task-navigation-responsiveness requirement/design `.12` through `.14`.
- Existing use-session-file-reviews ownership suite and real ReviewDialog.
- Durable parent diagnostic referenced by the manifest, not replayed as RED.

## Risks

Preserve rollback deletion, unrelated rows, reset barrier and retired direct
publishers without rejecting acknowledged actions. Keep write/write semantics.

## Results

Commands above ran with Node `24.21.0`, pnpm `9.15.9`:

- Frozen install passed (handle 29362, exit 0).
- Permanent causal RED before production edits: handle 54281, exit 1;
  10 expected state/checkbox assertion failures and 19 controls passed (29 tests,
  4.07 seconds). Pending/acknowledged cases are independently collected. Only
  deferred WebSocket transport is replaced; actual providers/protocol/consumer
  and classifier run. No setup error or timeout counted as RED.
- Final GREEN after the hook edit and test literal cleanup: handle 13504, exit 0;
  29 tests passed in 4.22 seconds, one worker and zero retries. Earlier GREEN
  handles 18312 and 17434 also passed and are terminal.
- Changed-file eslint passed with zero warnings (98379, exit 0). Two prior lint
  runs identified duplicated test literals; named constants resolved all of them.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed (60758,
  exit 0), including the unchanged rich-output catalog check. No guard changed.
- `pnpm run i18n:check` passed (20121, exit 0): keys, translations, Trans indices,
  plurals, module-scope calls, punctuation and non-JSX copy.
- Catalog validated 396 decisions / 1697 specifications; spec-linter tests passed
  all 36 tests; full specification lint and diff check passed. Design prose was
  condensed locally to meet the existing size cap, without a limit exception.
- Repository `validateCoverage` passed against all six actual source/test/docs
  paths: `covered`, `ok: true`, `errors: []`, with the owning requirement/design
  accepted. Exact invocation: Node 24 executing external `docs-coverage.cjs`
  from repo root; its source and result are preserved with the receipts.

Public docs need no change because the existing session-mark/hash and WS action
contracts already describe the restored behavior. Mobile parity uses the
source-proven state/data exception. No browser, live backend, full suite or Go
check is claimed. No operator runtime or shared seed was touched.

Main was refreshed independently at `49f8cd6377ec4025f94c4cad2955adf025f49de6`
before work. Later remote `719accef902ffd46a89175d75cb03c25e3f5448d` changes none
of the four owned existing files; parent retains integration judgment. Fresh
69-open-PR inventory plus full paginated PR 1512/4013 file patches found no
selected duplicate. No issue search or publication was performed.

Durable process/check/raw-diff/hook receipts live outside the checkout at
`/home/jcfs/.kandev/tasks/preserve-review-mark_3ari4908/receipts/attempt17/`.
The checkpoint commit must use normal active pre-commit and commit-msg hooks,
without bypass or amend; its exact SHA and hook verdict are recorded there.
Stop `ready_for_parent_validation` at a clean committed head before push/PR.
Parent actual-diff review and independent focused run precede separate explicit
same-session delivery authorization.
