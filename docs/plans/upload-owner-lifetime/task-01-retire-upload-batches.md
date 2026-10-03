---
id: "01-retire-upload-batches"
title: "Retire upload batches with their owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.7
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.8
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.9
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.10
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.6
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 01: Retire Upload Batches with Their Owner

## Summary

Bound uploads to the mounting UI owner and its session. Retire all pending phases without
dispatching another file, settle callers with cancellation and accurate write evidence, and
prevent stale conflict/state/toast publication while preserving current uploads.

## In scope

- `apps/web/hooks/use-file-upload.ts` and `apps/web/hooks/use-file-upload.test.ts`.
- New faithful `apps/web/components/task/use-file-upload-entry-points.test.tsx`, using real
  rendering/hook and mocked transport/toast. Immediate entry-point production glue only if
  this observable test establishes a reporting gap after the hook correction.
- Owning file-transfer requirement/design updates and this plan/work-order delivery record.
- Permanent RED then minimum GREEN, normal targeted validation and hooks, ready PR delivery
  under the [plan's completion gates](plan.md#verification-strategy-and-resources).

## Out of scope

Other source files, backend/API changes, generic lifecycle/abort frameworks, rollback, retry
policy, request-order changes, layout/copy/touch changes, extra work orders, agents/workers/
tasks/sessions, full local suites/build/E2E, foreign resources or edits, and routing/harness changes.

## Acceptance

1. Parked, preflight, direct-upload and conflict-resolution retirement obey `003.7` through
   `003.9`; caller settlement, real request counts and confirmed write/failure/skipped evidence
   are observable in deferred transport regressions. No next request or stale state/conflict
   publication follows disposal/session replacement.
2. Current uploads remain live under StrictMode replay, independent owners and replacement
   sessions; retained callbacks and stale success/failure finalizers cannot affect the new batch.
   Existing no-write-before-resolution, per-file choices/manual cancellation, empty/skipped,
   partial failure, session-change and current-success behavior remains covered.
3. Real mounted entry-point tests prove cancelled results do not toast and live success/failure
   still reports. Address any proven reporting continuation race with only local glue. Exact
   commands below pass with recorded counts/results; no permanent change before later release.

## Verification

Run commands from the repository root, independently rooted and one heavy command at a time.
Use the existing Node/pnpm PATH through `/bin/bash` with `login: false`; login zsh removes
those entries. Use the project-pinned pnpm 9.15.9 via `mise exec --` (both `mise.toml` and
`apps/package.json` pin it). Node 24.18.0 is the existing runtime.
Install once only if `apps/node_modules` or required workspace executables are absent:

```bash
(cd apps && NODE_OPTIONS=--max-old-space-size=4096 mise exec -- pnpm install --frozen-lockfile)
```

First add focused permanent lifetime regressions and run the affected hook/entry-point suites
to observe RED for the reported behavior. After minimum production correction, run the same
command for GREEN. Include existing conflict-dialog tests to preserve real caller interaction:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 mise exec -- pnpm exec vitest run --maxWorkers=1 hooks/use-file-upload.test.ts components/task/use-file-upload-entry-points.test.tsx components/task/file-upload-conflict-dialog.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 mise exec -- pnpm exec eslint --max-warnings 0 hooks/use-file-upload.ts hooks/use-file-upload.test.ts components/task/use-file-upload-entry-points.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 mise exec -- pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 mise exec -- pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 mise exec -- pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

If causal evidence requires entry-point production glue, add
`components/task/use-file-upload-entry-points.tsx` to the changed-file eslint command. Test
every changed suite together, with one worker. Do not repeat passing checks absent new changes
or a concrete failure. Retain and poll/join every command handle. The mobile-parity state/data
exception applies: real rendered entry-point tests cover result delivery; no geometry/browser
or new mobile Playwright test is required.

Use `.github/scripts/pr-docs.cjs`'s exported `validateCoverage` for the actual changed/untracked
file list and contents of this package plus its owning requirement/design. Require `ok: true`;
this checks REQ/AC/design/work-order/plan links beyond catalog and spec lint. At design handoff
leave artifacts unstaged/uncommitted and verify the new work order is visible in git status.
The docs-only design diff can be exempt; additionally validate the package against the planned
hook path to exercise its full cross-reference checks. This is documentation validation, not
production/test execution:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...execFileSync('git', ['diff', 'HEAD', '--name-only', '-z']).toString().split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const documents = [
  'docs/plans/upload-owner-lifetime/plan.md',
  'docs/plans/upload-owner-lifetime/task-01-retire-upload-batches.md',
  'docs/specs/ui/requirements/workspace-file-transfer.md',
  'docs/specs/ui/system-design/workspace-file-transfer.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [label, changedFiles] of [
  ['actual diff', paths],
  ['planned hook coverage', [...new Set([...paths, 'apps/web/hooks/use-file-upload.ts'])]],
]) {
  const result = validateCoverage({ changedFiles, fileContents });
  console.log(JSON.stringify({ label, ok: result.ok, status: result.status, errors: result.errors,
    workOrders: result.workOrders, references: result.acceptedReferences.length }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

During implementation update both Results records with actual commands/counts and run normal
commit hooks. No public copy, API, command, navigation or screenshot change is planned; internal
docs suffice for this lifecycle repair. Reassess only if implementation expands that impact.

## Files likely touched

- `apps/web/hooks/use-file-upload.ts`
- `apps/web/hooks/use-file-upload.test.ts`
- `apps/web/components/task/use-file-upload-entry-points.test.tsx` (new)
- `apps/web/components/task/use-file-upload-entry-points.tsx` (conditional causal glue)
- `docs/specs/ui/requirements/workspace-file-transfer.md`
- `docs/specs/ui/system-design/workspace-file-transfer.md`
- `docs/plans/upload-owner-lifetime/plan.md`
- This work order.

## Dependencies

None. One sequential task. The parent's later explicit implementation release in the same
session satisfied the checkpoint before permanent tests or production changes.

## Risks

See [plan risks](plan.md#risks). Preserve in-flight write evidence and isolate late finalizers.
The original parent proof is retained read-only; do not rerun/copy its whole harness. No
automatic audit, assertion weakening, retries, rebase or synthetic compatibility suite.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/workspace-file-transfer.md): `003.7` through
  `003.10` and preserved `001.5`, `004.1`, `004.3` through `004.6`.
- [Upload owner lifetime design](../../specs/ui/system-design/workspace-file-transfer.md#upload-owner-lifetime).
- Existing hook tests, real `FileBrowser` entry-point ownership, input/report/dialog flow.
- `hooks/use-prompt-result-delivery.ts` demonstrates instance-local generation fencing;
  use only as a local pattern, without importing or extending its unrelated public contract.
- Accepted parent RED receipts in [plan evidence](plan.md#evidence-and-root-cause).

## Results

Implemented after the parent's later explicit release in the same session.

- One frozen install from `apps/` with existing project-pinned pnpm 9.15.9: passed; no lockfile
  or package-manager changes. Node 24.18.0, non-login Bash, 4096 MiB cap.
- Permanent hook RED: parked unmount remained unsettled, preflight disposal dispatched a write,
  and focused active/stale-callback regressions had six assertion failures with four passing
  controls. Initial full RED exposed a test cleanup timeout cascade; that cascade is not counted
  as production evidence. All RED handles joined.
- Real entry-point completion-to-report RED: unmount still toasted after hook result completion,
  proving local glue necessary. The session variant explicitly commits replacement with
  `flushSync` to avoid testing a session render batched until after publication.
- Hook now uses instance-local session lifetime plus stable batch identity; cleanup retires the
  parked resolver, active continuations retain in-flight evidence and suppress late patches/
  requests, and old callbacks/finalizers cannot affect replacement work.
- Entry point now checks its captured reporting token after `uploadFiles`, preserving the result
  while suppressing stale toasts. No layout/copy/touch/API/backend changes.
- Exact three-suite GREEN: 52 tests passed. After removing three unsupported test-only
  `getByRole` options found by typecheck, the affected rendered suite passed all 12 tests;
  exact role-name semantics stayed unchanged. All targeted test handles joined.
- Changed-file eslint passed for all four source/test files; affected test lint passed after the
  typed-option correction. `pnpm run typecheck`, `i18n:check`, and `i18n:ratchet` passed via
  `mise exec`, with the 4096 MiB cap. No remaining owned check handle.
- Catalog validation passed (343 decisions, 1319 specifications); spec lint and diff check
  passed. Actual changed-file `validateCoverage` covered the one linked work order with no
  errors. Mobile parity uses the approved state/data rendered-test exception. Internal docs
  updated; public labels, APIs, commands, navigation and screenshots did not change.

Normal commit hooks and hosted review/merge are the remaining delivery gates, recorded in the
MCP task plan; task completion still requires actual verified merge and joined cleanup.

### PR review correction

The configured full all-eight-file CodeRabbit review and three Codex/Cubic threads identified
passive cleanup lagging committed retirement. Two real concurrent React/entry-point transport
regressions failed with preflight retirement sending one request instead of zero and upload
retirement sending two instead of one. A controlled scheduler clock yields at the commit
boundary; no hook or lifecycle function is mocked. Move the existing owner/reporting effects
to layout effects, preserving their settlement, reset and result-evidence behavior. Re-run the
exact three suites and affected lint/typecheck/i18n/docs checks. Published head may change only
for this actual corrective finding; no rebase, new work order, or scope expansion.

Correction results: exact three suites passed 54 tests; all four changed-file eslint, typecheck,
i18n check/ratchet, catalog (343 decisions/1319 specs), spec lint, diff check and actual coverage
passed. All owned correction check handles joined. Codex/Cubic threads and the grouped
CodeRabbit finding share commit-time invalidation; existing effects now use layout cleanup.
Normal corrective hooks/push, thread disposition and current-head hosted/full-review gates
remain delivery work in the MCP task plan.
