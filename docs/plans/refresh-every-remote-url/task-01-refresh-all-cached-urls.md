---
id: "01-refresh-all-cached-urls"
title: "Refresh all cached URLs in both native loaders"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.9
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.10
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.11
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.12
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 01: Refresh all cached URLs in both native loaders

## Summary

Correct both branch and inspection caches so every affected URL resolves after
provider availability changes. Prove the defect with permanent real-hook REDs,
then implement local invalidation and prove the native two-row outcome.

## In scope

- Both loader version gates, whole-cache invalidation and pending fencing using
  current request sequences, abort controllers and workspace guards.
- Real-registry hook and rendered native chips behavioral tests, as mapped in
  the plan. Include distinct first/second URL results and reversed ensure order;
  mixed settled success/error/empty, null inspection, replacement/removal and
  pending success/error/finalization without broad permutations.
- Preserve same-version sharing, trimmed keys, explicit retry, workspace and
  instance isolation, built-in fallback and structured plugin inspection.
- Synchronize owning specs, this work order and plan with actual results.

## Out of scope

Registry/SDK/backend/framework changes, exported API/cache shape changes,
committed row rewrites, passive-effect repairs, ToastProvider/poller/output
policy changes, public guide edits without actual contract impact, UI layout or
copy, browser/build/E2E, full suites, optional polishing or delegation.

## Acceptance

1. Permanent multi-URL production-hook tests fail for the second cached URL
   before the fix and pass for every URL afterward, independent of ensure order.
   The real rendered chips row automatically hydrates both pasted URLs and
   exposes their distinct native branch choices after provider boot.
2. Replacement/removal invalidates every affected settled/pending URL; old
   callbacks cannot replace current output or release current in-flight slots.
   Same-version per-URL sharing and explicit clear retry remain intact.
3. Existing normalization, workspace/instance isolation and provider fallback
   behavior pass the targeted suites. Every changed test path, documentation
   reference and production path is covered by actual final receipts.

## Files likely touched

- `apps/web/hooks/domains/github/use-branches-by-url.ts`
- `apps/web/hooks/domains/github/use-pr-info-by-url.ts`
- `apps/web/hooks/domains/github/use-branches-by-url.registry.test.ts`
- `apps/web/hooks/domains/github/use-pr-info-by-url.registry.test.ts`
- `apps/web/components/task-create-dialog-remote-repo-chips.provider-refresh.test.tsx`
- Existing `apps/web/hooks/domains/github/use-branches-by-url.test.ts` and
  `apps/web/hooks/domains/github/use-pr-info-by-url.test.ts` only if a missing
  preservation case requires it.
- Owning requirement/design pair and these two plan files for traceability and
  actual verification receipts. Consumer production source is read-only scope.

## Verification

Only after later explicit implementation release: read `/tdd`, mark this task
`in_progress`, install once if `apps/node_modules` is absent. Keep Node 24.21.0
first in PATH and pnpm 9.15.9 from `apps/package.json`; bash `login:false`
avoids the login shell replacing PATH. Never change lockfiles or wipe caches.

Run ONE heavy command at a time, each in a separate tool call. Each returned
`session_id` is live until `write_stdin` returns an actual terminal exit. Save
each command, handle, log and result before starting the next. Timeout,
nonzero exit even with zero reported issues, or crash-lost receipts are not
passes. No proof archive replay, passing repeat, broad verification or suite
shards. An unrelated failure requires exact leaf log/source evidence and
bounded parent direction, not retries or weakened assertions/checks.

Each command below is independently rooted from the repository root; use
`export PATH=/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH` in
each bash invocation. Run the focused command as RED after writing tests, then
as GREEN after production changes. Extend its explicit file list if any other
test path actually changes.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run --maxWorkers=1 hooks/domains/github/use-branches-by-url.test.ts hooks/domains/github/use-pr-info-by-url.test.ts hooks/domains/github/use-branches-by-url.registry.test.ts hooks/domains/github/use-pr-info-by-url.registry.test.ts components/task-create-dialog-remote-repo-chips.provider-refresh.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm exec eslint --max-warnings 0 hooks/domains/github/use-branches-by-url.ts hooks/domains/github/use-pr-info-by-url.ts hooks/domains/github/use-branches-by-url.registry.test.ts hooks/domains/github/use-pr-info-by-url.registry.test.ts components/task-create-dialog-remote-repo-chips.provider-refresh.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Add any actually changed existing test to the lint command. The existing hook
suites are already in Vitest scope. No new user-facing copy is expected.

PR documentation coverage preflight below reads actual unstaged/staged and
untracked paths without staging them. Run after implementation and again only
if the referenced delivery package changes. All four artifact contents are
loaded explicitly; errors must fail the command.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']).toString().split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0').filter(Boolean);
const paths = [...new Set([...tracked, ...untracked])];
const artifacts = [
  'docs/specs/plugins/requirements/repository-provider-task-creation.md',
  'docs/specs/plugins/system-design/repository-provider-task-creation.md',
  'docs/plans/refresh-every-remote-url/plan.md',
  'docs/plans/refresh-every-remote-url/task-01-refresh-all-cached-urls.md',
];
const fileContents = Object.fromEntries(artifacts.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles: paths.map(filename => ({ filename, status: 'modified' })), fileContents });
console.log(JSON.stringify(result, null, 2));
process.exitCode = result.ok ? 0 : 1;
NODE
```

Inspect actual changed paths against the explicit Vitest/lint lists, confirm
normal git hooks remain active, and record actual counts/exit receipts. Do not
mark this work order done from documentation checks alone. For mobile parity,
record the pure state/data exception and the real rendered consumer test result.

## Dependencies

None. Parent-supplied proof is accepted read-only. Completed
`plugin-repository-task-resolution` is background context, not a new dependency
or a package to reopen.

## Risks

Advance rather than reset sequences. Clear visible output as well as loaded
markers on removal. Preserve hook-local state and workspace epochs. Keep the
native integration faithful without production UI changes for test setup.

## Parallelism

`sequential`. Same primary session; no agents, extra tasks, tabs or model switch.

## Inputs

- [Owning requirements](../../specs/plugins/requirements/repository-provider-task-creation.md), AC .9-.12.
- [Native URL cache refresh design](../../specs/plugins/system-design/repository-provider-task-creation.md#native-url-cache-refresh).
- [Plan evidence and delivery checkpoint](plan.md).
- Both current hooks, existing hook suites, `RemoteRepoChipsRow`, real
  `RemoteRepoChip`/`Pill`, registry URL resolver and nearest rendered test.

## Results

Implemented after the explicit parent release on 2026-10-03. Both hooks now
invalidate every local cached/pending URL before per-URL deduplication, advancing
existing sequences before aborting. Provider and consumer production source,
exported shapes, layout and copy were unchanged.

The exact verification commands above completed sequentially with Node 24.21.0,
pinned pnpm 9.15.9, one Vitest worker and 4 GiB heap where specified:

- Frozen install: exit 0, joined handle 23241.
- Permanent regression RED: 11 expected failures/49 passing controls, joined
  handle 91757 exit 1. Both hook paths fail on the second cached URL, including
  reversed order, replacement/removal and pending checks. Real native beta row
  remains unhydrated before correction.
- Final five-suite GREEN: 60/60 tests pass, joined handle 12978 exit 0.
  Both actual native branch pickers select distinct feature branches after late
  provider boot. Real hooks/registry/row/chips retained; only transports mocked.
- Changed-file lint: zero warnings, joined handle 98012 exit 0.
- Typecheck: joined handle 67225 exit 0.
- i18n check and ratchet: joined handles 64290/30756 exit 0.
- Design catalog, 36 linter tests and all-spec lint remain valid for unchanged
  owning spec contents. Receipt details and early failing iterations are in
  the plan and task plan; no passing checks were repeated without changed input.

All three changed test paths are in the final exact focused Vitest command;
all five changed TS/TSX paths are in the exact zero-warning lint command. The
mobile-parity pure state/data exception is satisfied by the rendered outcome.
Actual changed-path coverage passed for all nine paths and one complete work
order, with whitespace/status clean (receipt `c616c3`). Normal commit hooks
validate their applicable gates without bypass.

Zero live handles after local verification. Owned logs remain under
`/tmp/kandev-child19-711b6d54/` until delivery receipts are preserved. Parent
proof archives remain read-only and parent-owned. Next action: normal commit,
push and ready PR; remote review/CI/actual merge proof and joined cleanup remain
pending. Local GREEN and this work-order completion do not complete the task.
