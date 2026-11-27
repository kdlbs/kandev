---
id: "01-current-workspace-reads"
title: "Correct both mounted diagnostic read lifetimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-LIVE-UPDATES-002
acceptance_criteria:
  - AC-OFFICE-LIVE-UPDATES-002.1
  - AC-OFFICE-LIVE-UPDATES-002.2
  - AC-OFFICE-LIVE-UPDATES-002.3
  - AC-OFFICE-LIVE-UPDATES-002.4
  - AC-OFFICE-LIVE-UPDATES-002.5
  - AC-OFFICE-LIVE-UPDATES-002.6
  - AC-OFFICE-LIVE-UPDATES-002.7
  - AC-OFFICE-LIVE-UPDATES-002.8
system_design:
  - ../../specs/office/system-design/live-updates-02.md
---

# Task 01: Correct both mounted diagnostic read lifetimes

## Summary

Replace each hook's lifetime-wide fetched latch with automatic reads for its
current committed selection. Fence all late publications using per-instance
selection and newest-request identity, verified through the real app store.

## In scope

- `useRoutingPreview` and `useProviderHealth` automatic/manual admission and
  success/error/finally ownership, including empty selection and unmount.
- One paired real-hook/real-StateProvider/createAppStore transport-only suite
  with the scenarios in the [manifest](plan.md#tests).
- Record exact results here and synchronize the manifest after implementation.

## Out of scope

Global caches/coordinators, shared generations or dedupe, cross-consumer
ordering, HTTP-versus-live-event arbitration, store/API/backend/schema/provider
registry/runner/broadcast changes, other Office hooks or component draft
lifecycle, routing policy, copy, layout, browser/build/E2E, and optional polish.
Preserve ROOT's archived proof unchanged. No implementation or permanent test
edits until the later ROOT reviewed-package implementation INTERRUPT.

## Acceptance

1. Both hooks automatically read each non-empty selection lifetime, suppress
   rerender/failure loops, expose neutral empty selection, and retain existing
   manual refresh with workspace-keyed data (`.1`, `.2`, `.3`, `.6`).
2. Selection/unmount invalidation and newest-read guards constrain data,
   errors, and loading completion, including pending success/failure,
   A-to-B-to-A and reverse-order manual refresh (`.4`, `.5`).
3. The real-hook/store suite proves independent instances/stores and all
   manifest scenarios. State-only desktop/phone parity is assessed explicitly
   (`.7`, `.8`); all exact bounded checks pass and all owned handles are joined.

## Implementation sequence

After ROOT releases implementation and the global local-heavy resource:
read the package and applicable AGENTS.md/TDD guidance, then mark this work
order `in_progress`. Use the existing Node binary and pinned pnpm; perform at
most one conditional frozen install if dependencies are absent. Write the
real-context settled-switch test first, run the RED command below and confirm
the missing beta read is causal. ROOT's archive is accepted evidence, not a
test to replay or overwrite. Complete deferred positive/failure/ownership
scenarios and make the smallest correction in both hooks. Run the GREEN and
changed-file gates once; repeat only after a real corrective change/finding.

Use the design's instance-local ownership, committed invalidation, and request
sequence. Existing store projections, result signatures, stable empty arrays,
and localized fallbacks remain. Do not copy the nearby mock-StateProvider
pattern. Actual `setActiveWorkspace` drives the selection tests; observe real
workspace-keyed entries. Resolve/reject every deferred transport and clean up
each consumer. Independent-instance tests must not invent a global newest
writer contract.

## Verification

Run from repository root in Bash with `login=false`, serially. These commands
are for the later implementation release only. Each timeout is a stop/checkpoint
bound, not permission to retry. Keep every returned session handle and join it
to terminal completion before starting the next operation.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"

# Check project-pinned pnpm before any package operation; expected 9.15.9.
(cd apps && timeout --signal=TERM --kill-after=5s 30s pnpm --version)
# One conditional install only after ROOT release/resource grant.
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 600s pnpm install --frozen-lockfile)
fi

# RED after adding the causal regression, before the production correction.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run hooks/domains/office/office-diagnostic-workspace-reads.test.tsx --project browser-locales --maxWorkers=1 --no-file-parallelism -t 'loads beta after alpha completes')

# GREEN after correction: all meaningful scenarios, one worker.
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run hooks/domains/office/office-diagnostic-workspace-reads.test.tsx --project browser-locales --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec eslint --max-warnings 0 hooks/domains/office/use-routing-preview.ts hooks/domains/office/use-provider-health.ts hooks/domains/office/office-diagnostic-workspace-reads.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 300s pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run i18n:ratchet --base 0eb74e57332900b7bd5ca756c541d16170000492)
timeout --signal=TERM --kill-after=5s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=5s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=5s 30s node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/office/requirements/live-updates.md',
  'docs/specs/office/system-design/live-updates-02.md',
  'docs/plans/office-diagnostics-workspace-refresh/plan.md',
  'docs/plans/office-diagnostics-workspace-refresh/task-01-current-workspace-reads.md',
];
const names = [
  ...execFileSync('git', ['diff', '--name-only', '0eb74e57332900b7bd5ca756c541d16170000492'], { encoding: 'utf8' }).trim().split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).trim().split('\n'),
].filter(Boolean);
const result = validateCoverage({
  changedFiles: [...new Set(names)].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
git diff --check
git status --short
```

The RED command intentionally exits nonzero for the causal assertion; other
failures do not count as RED. Typecheck uses the established web command
(including its normal generation hook), with a finite process bound; there
is no standalone scoped TypeScript project. Inspect any resulting generated
diff rather than discarding foreign files. A timeout, installation mismatch,
resource failure or unrelated diagnostic checkpoints ROOT; do not add broad
passing tests, new retry budgets or bypass active hooks.

For documentation traceability, call the existing
`.github/scripts/pr-docs.cjs::validateCoverage` against the actual changed
paths and linked file contents during later pre-publication checks. The design
receipt separately validates the anticipated hook paths against this actual
four-file documentation package; it is not runtime verification.

## Files likely touched

- `apps/web/hooks/domains/office/use-routing-preview.ts`
- `apps/web/hooks/domains/office/use-provider-health.ts`
- `apps/web/hooks/domains/office/office-diagnostic-workspace-reads.test.tsx` (new)
- This work order and `plan.md` for actual delivery results.
- Preserve existing draft statuses of the broader migrated requirement/design;
  record this amendment's completion here without asserting unrelated legacy
  criteria are implemented.

Read-only integration inputs: `components/state-provider.tsx`,
`lib/state/store.ts`, `lib/state/slices/office/office-slice.ts`,
`lib/api/domains/office-routing-api.ts`, `src/office-routes.tsx`, all consumers
listed in the manifest, and the existing WS health handler. No lockfile or
dependency change is planned.

## Dependencies

None. ONE sequential work order; execution requires the later ROOT release
and global local-heavy grant. No delegation, new task/session/tab or profile
change is authorized.

## Mobile and public docs

The existing phone Office workspace picker and desktop picker reach the same
state path. This correction changes data/request lifetime only; the mobile
parity state-only exception is satisfied by targeted real-context tests and
the manifest assessment. No new mobile Playwright, rendered preview, control
or copy is needed. Public-doc audit found only existing in-progress Office
and dynamic-routing guidance; no public documentation change is required.

## Risks

Same-name re-entry and stale `finally` are easy to miss. A valid failure must
not erase cached data or admit a rerender retry loop. Scope guards must be
per instance/store and invalidate only that consumer. Shared-event ordering
and deprecated routing migration stay outside this task.

## Parallelism

`sequential`

## Inputs

- [REQ-OFFICE-LIVE-UPDATES-002](../../specs/office/requirements/live-updates.md#req-office-live-updates-002-current-workspace-diagnostic-reads), criteria `.1` through `.8`.
- [Current-workspace diagnostic read design](../../specs/office/system-design/live-updates-02.md#current-workspace-diagnostic-reads).
- [Manifest evidence, consumers, test matrix and gates](plan.md).
- ROOT proof receipt/archive/log and durable Kandev task plan with task/session
  IDs, system marker, resource ownership and later delivery/merge gates.

## Results

ROOT released this reviewed package and its exclusive global local-heavy slot.
One conditional frozen pnpm9.15.9 install completed. Only the two production
hooks and one transport-mocked, real-StateProvider/createAppStore test file
were changed. Committed layout ownership invalidates departed callbacks and
requests; newest-read identity guards success, catch, and finally. No global
coordination or public result/store/API change was introduced.

The exact RED command above exited 1 with two missing-beta-read assertions
after real alpha publication. The expanded baseline run exited 1 with 28
semantic failures and 18 passes. An early own-fixture deferred-await timeout
was repaired and was not counted as RED evidence. Root StrictMode replay uses
the testing-library root option, exercising actual cleanup/setup.

The GREEN command passed all 46 tests, including current/empty reads, real
switches, pending success/failure, manual races, independent consumers/stores,
unmount, and StrictMode replay. Changed lint passed after splitting an oversized
test registration; typecheck and both i18n commands passed. The final test run
also asserts neutral empty state immediately and no departed store write.
Catalog validation passed (351 decisions, 1368 specifications). Specification
lint passed; actual-path coverage returned `covered`, `ok: true`, `errors: []`
for the seven changed files. All completed handles were joined;
exact chunks, versions, remaining resources, and later PR/head/CI evidence are
kept in the durable task plan rather than fixed into published docs.

State-only mobile assessment and public-doc audit remain applicable. No browser,
build, E2E, layout, copy, backend, schema, registry, or lockfile changes. ROOT's
archived proof remains untouched. Normal hooks/publication follow scoped gates;
exact-head CI/review and separate ROOT serial merge remain external gates.
