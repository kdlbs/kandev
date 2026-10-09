---
id: "01-retire-obsolete-searches"
title: "Retire obsolete chat searches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-SEARCH-OWNERSHIP-001
acceptance_criteria:
  - AC-UI-SESSION-SEARCH-OWNERSHIP-001.1
  - AC-UI-SESSION-SEARCH-OWNERSHIP-001.2
  - AC-UI-SESSION-SEARCH-OWNERSHIP-001.3
  - AC-UI-SESSION-SEARCH-OWNERSHIP-001.4
system_design:
  - ../../specs/ui/system-design/session-search-ownership.md
---

# Task 01: Retire Obsolete Chat Searches

## Summary

Correct local search ownership so only an eligible current query/session/lifetime
can admit requests or publish settlement. Add meaningful regression coverage with
the rendered production hook, real API and real WebSocket protocol client.

## In scope

- TDD for query, close/reopen, committed session/null/unmount and retained callback
  boundaries, including success/failure/finalizer and independent-instance controls.
- Local hook correction and preservation of debounce/wire/current-error/newer-request
  ordering and existing navigation/MAX40 raw-backfill behavior.
- Synchronize the four package files with actual results after bounded checks;
  promote draft requirement/design only when matching implementation is verified.

## Out of scope

All exclusions in the [plan](plan.md#scope) apply. No consumer production changes
without causal evidence and a scope checkpoint; no new exported production seam,
coordinator, transport cancellation, backend, public docs, framework, or extra task.

## Acceptance

1. The real-protocol regression proves `.1` through `.3`, including no unwanted
   wire admission and no stale mutation in success/catch/finally paths.
2. `.4` controls prove current searches, exact wire identity, 180ms debounce,
   failures, newer ordering, independent lifetimes and existing navigation/backfill.
3. Every bounded check below is actually joined and recorded. Only the approved
   hook/tests and four artifacts change; task status and spec lifecycle are truthful.

## Verification

Do not execute until a LATER ROOT implementation INTERRUPT in this same primary
session and an explicit sole global-heavy lease. All commands below run from
repository root via `/usr/bin/bash` with login=false. Use existing Node24 with
command-local environment only; no shell profile or runtime configuration changes.
Record start/deadline/log/PID/process-group/returned handle for every started
command and actually join before the next. Bounds: install 600s, each targeted
Vitest pass 120s, ESLint 120s, typecheck 300s, each i18n gate 120s, each docs gate
60s. An owned watchdog may terminate only proved owned processes at the bound;
checkpoint ROOT after joining and proving cleanup. No automatic replacement or
resource increase. Short design validation is separate from these future checks.

### Environment and conditional install

```bash
export PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:/usr/local/bin:/usr/bin:/bin
export NODE_OPTIONS=--max-old-space-size=4096
node --version
if [ ! -d apps/node_modules ] || [ ! -d apps/web/node_modules ]; then
  (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

Only one conditional install is admitted. Dependencies are currently absent.
If that attempt fails or times out, checkpoint; no retries, cache wipes or
alternative installs. Retain the managed dependencies after completion.

### Red, then green

After adding the causal regression, run the new suite once before production
edits and classify causal failures separately from fixture/transport failures.
The original ROOT proof remains read-only and is never replayed.

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/session/use-session-search-ownership.test.tsx --maxWorkers=1 --no-file-parallelism)
```

After the smallest hook correction, run both owned suites together:

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/session/use-session-search-ownership.test.tsx hooks/domains/session/use-session-search.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && corepack pnpm@9.15.9 exec eslint hooks/domains/session/use-session-search.ts hooks/domains/session/use-session-search.test.ts hooks/domains/session/use-session-search-ownership.test.tsx --max-warnings 0)
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)
```

Run one typecheck. Its established pretypecheck generation is part of that
command; inspect outputs and preserve normal hooks. No unrelated passing replays,
broad suites, backend/browser/build/DB commands or generic verification phase.
Routine causal owned fixture/lint corrections rerun only affected gates; never
weaken assertions/races/timing or reinterpret setup failure as causal evidence.

### Documentation and whitespace

```bash
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py specs --system ui --text session-search-ownership --format paths
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Run this exported coverage preflight with current document bytes and the bounded
implementation path inventory. No network or status publication is needed.

```bash
node - <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/ui/requirements/session-search-ownership.md',
  'docs/specs/ui/system-design/session-search-ownership.md',
  'docs/plans/session-search-ownership/plan.md',
  'docs/plans/session-search-ownership/task-01-retire-obsolete-searches.md',
];
const source = [
  'apps/web/hooks/domains/session/use-session-search.ts',
  'apps/web/hooks/domains/session/use-session-search.test.ts',
  'apps/web/hooks/domains/session/use-session-search-ownership.test.tsx',
];
const result = validateCoverage({
  changedFiles: [...documents, ...source].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

Record actual changed paths separately; the preflight inventory checks package
traceability even if an existing test file does not need edits. Public guidance has no affected query
lifetime instructions, API/labels/navigation/screenshots or coverage owner change;
no public file edit is planned. Checkpoint any demonstrated authoritative gap.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-search.ts`
- `apps/web/hooks/domains/session/use-session-search.test.ts`
- `apps/web/hooks/domains/session/use-session-search-ownership.test.tsx`
- The four design-package files referenced by this work order and plan.

## Dependencies

No prior work order. Actual-file ROOT review, later implementation INTERRUPT,
and sole global-heavy release are required. No delegates or extra platform tasks.

## Risks

See [ownership risks](plan.md#risks). No API/WS/TaskChatPanel replacement mock can
substitute for the transport-only production-hook regression. Tests must restore
owned wire globals/client and finish fixture cleanup, including after failures.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/session-search-ownership.md).
- [Design and exact source inventory](../../specs/ui/system-design/session-search-ownership.md).
- Accepted historical ROOT proof and receipt paths in the plan, read-only.
- Scoped web/chat AGENTS.md, `/tdd`, mobile-parity state/data exception and public
  documentation guidance. Read delivery skills only when released delivery begins.

## Results

Completed after ROOT's later explicit same-primary implementation release and
exclusive local-heavy grant. Accepted design hashes remain in ROOT's review
receipt and the Kandev task plan; source edits began only after clean causal RED.

| Required check | Actual result |
| --- | --- |
| Conditional `corepack pnpm@9.15.9 install --frozen-lockfile`, apps | Passed once; managed dependencies retained |
| New real-protocol suite on original hook, browser-locales/one worker/no file parallelism | Clean causal RED: 14 failed, eight controls passed; initial owned async-act fixture issue corrected before this run |
| Final two-suite Vitest command above | 29 passed across two files; 22 new and seven existing tests |
| Changed ESLint command above, max-warnings 0 | Passed after splitting the owned test's oversized outer describe; no rule suppression |
| One normal `pnpm run typecheck` | Passed, including established pretypecheck generation |
| `pnpm run i18n:check` and `pnpm run i18n:ratchet` | Passed; no new UI copy or locale changes |
| Catalog validate and full spec lint | Passed; focused catalog identifies both owned specifications |
| `node scripts/validate-public-docs.mjs` | 47 published pages passed |
| Exported `validateCoverage` with actual six-file diff | Covered with no errors and complete linked requirement/design/plan/order |
| `git diff --check`, tracked/untracked whitespace and actual status | Passed; only hook/new test/four artifacts changed |

Final product inputs passed after fixture grouping and ordinary formatting, with
no later source/test edit. Existing navigation tests required no edits; the new
suite additionally preserves MAX40 backfill and close cancellation. The actual
coverage invocation uses owned `/tmp/kandev-child44-search-tpc2n89y/coverage.cjs`
to derive changed paths from raw Git output and call the exported validator;
result is in `coverage-result.json`. Exact starts/deadlines/PIDs/groups/handles/
logs and joined exits are recorded in the task plan and owned evidence directory.
No command timed out; all owned local groups are gone. No browser/E2E/build or
broader suite ran, and the original ROOT proof was not replayed or removed.

Local implementation is complete. Publication, hosted CI/review, heavy-lease
return, and a separate ROOT merge lease remain governed by the plan and live task
checkpoint. A completed work order is not task/delivery completion.
