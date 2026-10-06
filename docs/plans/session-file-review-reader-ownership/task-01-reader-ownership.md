---
id: "01-reader-ownership"
title: "Enforce mounted file-review reader ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.12
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.13
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.14
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 01: Enforce Mounted File-Review Reader Ownership

## Summary and inputs

Implement the local reader lifetime boundary and one faithful regression suite
after a later explicit ROOT implementation release in the same primary.
Read the [manifest](plan.md), [requirement](../../specs/ui/requirements/task-navigation-responsiveness.md),
[mounted reader design](../../specs/ui/system-design/task-navigation-responsiveness.md#mounted-file-review-reader-ownership),
`apps/web/AGENTS.md`, and `/tdd` before changing code. The existing
`hooks/domains/session/use-session-search-ownership.test.tsx` shows real deferred
wire conventions; do not copy its unrelated behavior. ROOT's original proof
resources and compatibility receipt are listed in the manifest and remain read-only.

## Scope and files likely touched

- `apps/web/hooks/use-session-file-reviews.ts`: session/lifetime snapshot,
  committed retirement, guarded deferred/event/direct local publication.
- `apps/web/hooks/use-session-file-reviews.test.tsx`: one suite containing
  hook controls and the mounted real Review consumer assertion. Real
  `StateProvider`, `ToastProvider`, and `TooltipProvider` as required; real
  connection singleton/WebSocketClient and classifiers, only wire deferred.
- The two owning specification files and these two plan files: record truthful
  status and final results without changing sibling packages.

Minimal immediate consumer test glue is conditional on an actual causal need
and stays in this suite where possible. No production consumer edit is expected.
Exclude all residual boundaries in the manifest, particularly mutation admission
and ordering, cache/framework migration, and transport cancellation.

## Acceptance

1. Permanent RED reproduces both admitted hook failures through real protocol
   requests and actual hash/classification. The mounted dialog's actual checkbox
   or progress shows the same Beta-owned outcome with its reader retained.
2. Commit-phase retirement and pure projection satisfy `.12`/`.13`, including
   deferred work, A-to-B-to-A, StrictMode and unmount, while proper-session cache
   publication and all `.14` current/shared/optimistic controls remain intact.
3. Run every required bounded command below, record actual exits, logs and joins,
   synchronize this order and manifest, and pass the actual documentation
   coverage oracle. Do not claim implementation complete from historical proof.

## TDD and consumer proof

Name the suite `session file review reader ownership` so the anchored selector
includes every test. First reproduce late Alpha after Beta false with identical
`reviewFileKey` and real `hashDiff`, and completed Alpha to committed null.
Record expected assertion failures, then implement and run GREEN once. Add the
design's deferred ownership/control matrix before final GREEN.

For mounted proof render the real `ReviewDialog` with ready, identical file data
and real provider state, auto-mark disabled through settings. Rerender non-null
session props on that mounted dialog; do not key it by session. Verify mount
continuity and query the real `review-file-row` checkbox for the exact path,
then settle replies by wire action/session/request ID. The keyed inner row may
remount normally. Keep underlying build/compute/hash logic real. If the real
dialog needs unrelated machinery that cannot be bounded, checkpoint ROOT with
the concrete limitation; do not silently substitute a synthetic consumer.

Use unique test session IDs, settle all deferred reads/mutations and any
consumer-started wire actions, disconnect, restore previous singleton and
providers/globals, and drain/clear owned fake timers. No production cache reset
export, broad component mocks, network replay, wall-clock waits or setup-error
RED. Current mark failure keeps its existing rollback semantics; no adversarial
mutation ordering inference is authorized by the read proof.

## Admission and bounded processes

DESIGN ONLY until ROOT releases this package and grants heavy work. Re-read
relevant source and package/user edits on admission. If relevant source differs
from the audited checkpoint, persist evidence and checkpoint ROOT before RED.
Do not reset/rebase to the old proof, install now, or launch delegates/sessions.

Run each command from repo root in `/bin/bash` with `login=false`. Use existing
Node 24 at `/home/jcfs/.nvm/versions/node/v24.18.0/bin` and pinned pnpm 9.15.9.
One conditional frozen install from `apps/` is permitted only later if local
dependencies are absent/unusable; no alternate lockfile/package-manager path.

For every admitted process, persist command/cwd/start/cutoff/log and wrapper
PID/child PID/process group before waiting. Launch in an owned process group
with `timeout --kill-after=5s <cap>s`, retaining each returned native handle.
Poll handles to actual completion (tool waits at most 60 seconds); record exit
and descendant/group settlement, not merely an early return. Cap install at
300s, Vitest/ESLint at 120s, typecheck at 180s, i18n at 120s, docs at 60s.
On timeout/resource/transport/unknown/out-of-scope failure persist a ROOT
checkpoint and END WAITING; no automatic replacement or resource retry.
Routine causal own fixture, CLI selector or coverage arguments may be corrected
and rerun affected-only within the grant. No stale PASS or duplicate unknown
mutation. We are not alone: preserve foreign processes and artifacts.

## Verification

Commands below are command payloads for that bounded runner. Each directory is
independent. No browser/build/E2E/backend/full suite has a causal requirement.

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
export NODE_OPTIONS=--max-old-space-size=4096

# Conditional later install only, 300s; confirm dependency usability first.
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)

# 120s, once for causal RED and once for final GREEN after the completed change.
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/use-session-file-reviews.test.tsx -t '^session file review reader ownership ' --maxWorkers=1 --no-file-parallelism)

# 120s, affected files only. Add any actually changed immediate test glue explicitly.
(cd apps/web && corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/use-session-file-reviews.ts hooks/use-session-file-reviews.test.tsx)

# 180s; normal package pretypecheck, no repeated broad pass.
(cd apps/web && corepack pnpm@9.15.9 run typecheck)

# 120s each, from web package scope.
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)

# Light docs gates, 60s each; also run during design.
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/ui/requirements/task-navigation-responsiveness.md docs/specs/ui/system-design/task-navigation-responsiveness.md docs/plans/session-file-review-reader-ownership
git status --short -- docs/plans/session-file-review-reader-ownership
```

Run the real local coverage oracle in 60s from repo root, with the actual
unstaged/staged/untracked paths (no pretend triggering code or synthetic checks):

```bash
node <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const split = value => value.split('\0').filter(Boolean);
const paths = [...new Set([
  ...split(execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' })),
  ...split(execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' })),
])];
const documents = [
  'docs/specs/ui/requirements/task-navigation-responsiveness.md',
  'docs/specs/ui/system-design/task-navigation-responsiveness.md',
  'docs/plans/session-file-review-reader-ownership/plan.md',
  'docs/plans/session-file-review-reader-ownership/task-01-reader-ownership.md',
];
const fileContents = Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles: paths.map(filename => ({ filename, status: 'modified' })), fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
// Docs-only design is legitimately exempt. Invoke the existing private
// work-order validator read-only, without adding a synthetic triggering path.
const filename = path.resolve('.github/scripts/pr-docs.cjs');
const context = { require: createRequire(filename), module: { exports: {} }, process, console, Buffer, URL, fileContents, workOrderPath: documents[3] };
vm.runInNewContext(fs.readFileSync(filename, 'utf8') + '\nglobalThis.validation = validateWorkOrder(workOrderPath, fileContents, new Set());', context, { filename });
console.log(JSON.stringify(context.validation, null, 2));
if (context.validation.errors.length) process.exitCode = 1;
if (paths.some(path => path.startsWith('apps/web/')) && result.status !== 'covered') process.exitCode = 1;
NODE
```

This invokes the unchanged repository validator in a read-only VM to reach its
private work-order function. It performs no tests, mutations, GitHub calls or
synthetic changed-path classification. Record design `exempt` separately from
the direct reference result; implementation must produce real `covered` evidence.

If concurrent foreign changed work orders need additional references, load their
existing document closure into this oracle read-only and preserve their edits;
never fabricate a filtered passing verdict. Inventory owned/unowned paths before
staging later, and exclude generated/typecheck collateral unless actually owned.

## Mobile and public docs

The [manifest audit](plan.md#mobile-e2e-and-public-documentation) applies the
mobile-parity pure state/data exception. No layout preview or new mobile E2E.
Public session-owned review and unchanged WS action docs already cover the
intended behavior; no public edit or public validator is needed for this package.

## Dependencies, parallelism and delivery

Dependencies: None. Parallelism: `sequential`, same primary/profile, no delegates.
Normal hooks only; formatter correction creates a new commit, never amend/bypass.
Later standing delivery authorization and ROOT's separate serial MERGE gate are
in the manifest and live task plan. No commit/push/PR this design turn. No final
completion until verified merge and joined or explicitly crash-reconciled cleanup.

## Risks and results

Lifetime-only filtering must not block proper-session cache notifications or
change mutation cache ordering. The test must prove a retained real reader and
avoid incidental automatic marking. Implementation released after ROOT reviewed
the completed design checkpoint. The local owner snapshot/commit retirement and
guarded publisher are implemented without production consumer edits. See the
manifest for permanent causal RED, real mounted dialog proof, historical owned
fixture/control/lint/typecheck failures, and final 17-test/lint/typecheck/i18n
passes. Final catalog/spec lint and actual six-path documentation coverage passed
(`covered`, `ok: true`, `errors: []`), with accepted owning requirement/design and
direct work-order reference validation. Delivery and ROOT merge gates remain
external. Accepted ROOT proof remains read-only.
