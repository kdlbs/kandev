---
id: "01-recover-empty-search"
title: "Correct recovered empty searches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-WORKSPACE-CONTENT-SEARCH-001
acceptance_criteria:
  - AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.10
system_design:
  - ../../specs/ui/system-design/task-workspace-content-search.md
---

# Task 01: Correct Recovered Empty Searches

## Summary

Successful retry responses supersede earlier errors, including empty responses.
Prove the real hook/WS boundary RED before the minimal production correction.

## In scope

Hook recovery, targeted regressions, actual consumer outcomes and owning docs.

## Out of scope

Backend retries, caches, committed-render races, plugin settings, publication.

## Acceptance

- Recovered initial/middle failures finish with no matches and no error.
- All-failed and late terminal failure without matches remain transport errors;
  mixed collected matches survive, with existing timing and cancellation intact.
- Real hook and wrapper run with only the request transport mocked; consumer
  selects the existing translated no-match/failure row.

## ASCII UI preview

### UI-01: Existing result row

See [the shared desktop/phone preview](plan.md#ui-01-existing-result-row).
Recovered empty: `[No content matches found.]`; final failure without matches:
`[Search failed. ...]`. No mobile composition or input changes.

## Verification

Run from repository root, one bounded worker and no full suite overlap:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run --maxWorkers=1 hooks/domains/session/use-workspace-content-search.test.ts hooks/domains/session/workspace-content-search-recovery.test.tsx components/workspace-content-search.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-workspace-content-search.ts hooks/domains/session/use-workspace-content-search.test.ts hooks/domains/session/workspace-content-search-recovery.test.tsx)
(cd apps/web && NODE_OPTIONS="${NODE_OPTIONS:-} --max-old-space-size=6144" pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/workspace-content-search-recovery
```

After committing, run PR documentation coverage against that actual head:

```bash
node <<'JS'
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const fields = execFileSync('git', [
  'diff', '--name-status', '-z',
  '6b7ffd69d8120f74b133072f0ea87f15e31c7e3e', 'HEAD',
]).toString().split('\0');
fields.pop();
if (fields.length % 2) throw new Error('Unexpected diff framing');
const changedFiles = [];
for (let i = 0; i < fields.length; i += 2) {
  if (!['A', 'M'].includes(fields[i])) throw new Error('Unexpected diff status');
  changedFiles.push({ filename: fields[i + 1], status: fields[i] === 'A' ? 'added' : 'modified' });
}
const fileContents = Object.fromEntries(changedFiles
  .filter(({ filename }) => filename.endsWith('.md'))
  .map(({ filename }) => [filename, execFileSync('git', ['show', `HEAD:${filename}`]).toString()]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
process.exitCode = result.ok ? 0 : 1;
JS
```

## Files likely touched

- `apps/web/hooks/domains/session/use-workspace-content-search.ts`
- `apps/web/hooks/domains/session/use-workspace-content-search.test.ts`
- `apps/web/hooks/domains/session/workspace-content-search-recovery.test.tsx`
- `docs/specs/ui/requirements/task-workspace-content-search.md`
- `docs/specs/ui/system-design/task-workspace-content-search.md`
- `docs/public/sessions-and-review.md`
- This plan and work order.

## Dependencies

None. No delegated work.

## Risks

A successful early attempt must not hide a later terminal failure without matches.

## Parallelism

`sequential`

## Inputs

Owning requirement AC .10, design Browser retry recovery, existing hook lifecycle
and consumer label tests. Parent real-hook diagnostic: 2 expected failures and
2 passing controls on unchanged source; child must independently reproduce RED.

## Results

- Frozen dependency install: passed (934 packages, lockfile unchanged).
- Causal RED on unchanged production: 41 tests, five expected failures and
  36 passing controls. Initial/middle/last-attempt recovery and recovery cadence
  ended in transport-error; the actual translated consumer rendered failure.
- Final exact three-suite Vitest command above: 41/41 tests passed, 3/3 suites,
  one worker, 2.14 seconds. Only request transport is mocked in the recovery
  chain; actual hook, WS wrapper, consumer, state provider and command primitives run.
- Focused ESLint command above: passed, zero warnings. Initial repeated-string
  and oversized test-group warnings were corrected without changing production.
- Typecheck command above: passed with a 6 GiB Node heap inside the unchanged
  8 GiB build guard. The initial default 4 GiB run exhausted its JS heap and
  exited 134; its following test command did not run. No core file remained.
- Catalog validation: passed (396 decisions, 1691 specifications).
- Specification unit tests: 36 passed; full specification lint: passed.
- Public-doc validator tests: 63 passed; public docs: 54 pages validated.
- Filesystem PR documentation coverage preflight: covered, no errors.
  The committed-head command above is repeated for the parent handoff receipt.
- Diff whitespace and plan path checks: passed. All new work orders inspected.
- Existing owners were reused, with minimal AC .10 clarification and browser
  recovery design. Public docs updated: `docs/public/sessions-and-review.md`
  (task-search reference within the existing usage guide).
- Mobile-parity state-only exception applies: no viewport-dependent logic,
  layout, navigation, touch or scrolling changes. Browser desktop/phone and
  actual remote-backend checks were not run or claimed. Consumer evidence is
  happy-dom integration, without the surrounding CommandPanel or live backend.
- Fresh duplicate checks: main remains `6b7ffd69d8120f74b133072f0ea87f15e31c7e3e`;
  PR 1512 at `a8655ad92faa050a68140a904500fc99ebc0a04b` and PR 3113 at
  `f5cfc7e228390f3e2065af78641a260fd0009464` have no exact target-file matches.
  No issue backlog was searched. No live incident is claimed.

The production diff only clears the prior error on a successful response.
Publication requires parent diff inspection and separate explicit authorization.
