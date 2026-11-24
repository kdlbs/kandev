---
created: 2026-10-22
status: done
requirements:
  - REQ-INTEGRATIONS-LINEAR-WATCH-DIALOG-001
system_design:
  - ../../specs/integrations/system-design/linear-watch-dialog-lifetime.md
legacy_specs: []
---

# Implementation Plan: Linear watch dialog ownership

## Overview

Repair awaited unconditional close/finally ownership in the retained Linear
watch dialog. Supplied immutable-source transport proof has one causal failure
and one owned-success control. The user explicitly authorizes package and TDD
in this session; publication remains a separate parent authorization.

## Scope

Own dialog completion, dismissal and busy state plus local tests/helpers.
Exclude connection settings, all other providers, polling, shared caches,
backend/API, schema and unrelated producers.

## Technical approach

Implement the paired design within `linear-issue-watch-dialog.tsx`. Keep the
real section/domain/API/store/toast path unchanged. Use mounted consumer tests
with only HTTP and unrelated Monaco renderer adapted. First reproduce against
unchanged source; then isolate opening and create selection ownership.

## ASCII UI preview

UI-01: Install-wide Linear settings, current create/edit opening (desktop and phone).
The existing dialog scrolls vertically; field order and footer stay unchanged.
Before: old ACK dismisses the reopened dialog. After: later draft stays open.

```text
[Linear watchers]                         [New watcher]
[watch row] -> [Workspace choice (edit locked)]
              [Filter: later raw query       ]
              [Automation / Prompt / Settings]
              [Cancel] [Update/Create/Saving]
```

The same composition is retained on phone with native taps and the shipped
scroll owner. Spacing is illustrative. Ownership and usable actions are required
by AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.1 through .6.

## Tests

`components/linear/linear-watch-dialog-lifetime.test.tsx` protects all criteria:
held update/create success/failure, same/different/create replacement, current
error/retry, newer busy, original request scope, explicit create scope switching,
passive hydration, StrictMode, unmount and abandoned suspended render.

## E2E tests

`e2e/tests/integrations/linear-watch-dialog-ownership.spec.ts` (chromium) and
`mobile-linear-watch-dialog-ownership.spec.ts` (mobile-chrome) exercise real
accepted update/create plus later Save/Cancel. A deliberate failure control
proves cleanup. Full raw source/seed HEAD and porcelain guards surround flows.

## Work orders

- [x] [Task 01: Preserve current watch](task-01-preserve-current-watch.md)

## Verification

```bash
export PATH=/home/jcfs/.local/share/pnpm/.tools/pnpm/9.15.9/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/linear/linear-watch-dialog-lifetime.test.tsx components/linear/linear-issue-watch-form.test.ts components/linear/linear-issue-watch-dialog.test.tsx components/linear/linear-issue-watchers-section.test.tsx)
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/integrations/linear-watch-dialog-ownership.spec.ts -- --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/integrations/mobile-linear-watch-dialog-ownership.spec.ts -- --retries=0
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run changed-file ESLint/Prettier and PR documentation coverage preflight using
its actual CLI. Retain external raw logs, RED/control/GREEN, resource teardown,
full diff/hash and normal active hook receipts. Recheck open PR inventory and
actual relevant patches at checkpoint. Stop before push/PR.

## Verification results

Completed in the authorized primary session. No subagents, push or PR.

- Causal unchanged-source RED: exactly one intended missing-reopened-dialog
  failure and one ordinary owned-save control passed (945 ms tests, 8.45 s total).
  Original PATCH scope, real store/table publication and truthful toast were
  asserted before the failure. pnpm setup failures are recorded separately.
- Mounted consumer GREEN: 52 tests passed across four focused suites. The 28
  lifetime schedules use real State/Toast/Tooltip/SettingsSave providers,
  install-wide section/domain/API/store and held HTTP; only the unrelated
  Monaco renderer is adapted. Includes both current create/update failure/retry,
  raw cap and provider/destination switch retention/reset, pending later saves,
  original workspace/ID/payload, same-ID hydration, passive active/default
  hydration, StrictMode, unmount and an uncommitted suspended replacement.
- Fresh managed production desktop: 3 passed (14.6 s). Fresh managed phone:
  2 passed (16.8 s), using Pixel 5 native taps. Both use one shard/worker,
  retries 0, strict WS guards, GOMAXPROCS=2 and GOFLAGS=-p=2. HTTP responses
  were held after real backend acceptance; later draft/focus/caret and
  Save/Cancel, real rows/toasts and reload persistence passed.
- All five browser cleanup receipts have no cleanup errors. The deliberate
  primary failure was expected. Full raw HEAD/porcelain guards cover this code
  checkout, immutable owned worker seed and owned source before/during/after.
  Created workspaces/watches and Worktree profiles were explicitly deleted;
  both managed backend roots and all owned source/seed roots are absent.
- Changed-file ESLint passed. Typecheck uses the documented 8 GiB Node budget.
  i18n/catalog/ratchet, catalog validation, all-spec lint and 36 linter tests,
  public-doc validators (63 tests, 54 pages), discovery (zero errors) and PR
  documentation coverage passed. Final typecheck and normal active hook
  receipts are retained in the external packet before handoff.
- Public docs assessment: reference/how-to integration guidance needs a small
  clarification; only the existing Linear watch paragraph changed. Landed
  Linear connection, Sentry, GitLab and Jira text remains byte-preserved.
- Duplicate assessment: all-paginated relevant PR file inventories (895 files
  across 9 pages for #1512, 27 for #2336, 19 for #4375, 33 for #4584) and actual
  target patches show no dialog lifetime fix. Their heads were rechecked.
  The refreshed open inventory is 69; this is a bounded target assessment.

Evidence, all terminal command handles, setup/fixture failures, original source,
pre-TDD package timestamp/hash/copies, full final diff/hash and hook/resource
receipts live outside the repository at
`/home/jcfs/.kandev/tasks/preserve-newer-linea_28ddw2qj/linear-watch-receipts`.
Browser evidence is emulated and mock-provider backed; no physical-device,
OS/operator-runtime or zero-flake claim is made. This package adds no backend,
connection-settings, polling/cache authority or server-ordering change.

## Risks

Create scope changes must preserve `switchWatchWorkspace` semantics. Passive
hydration and abandoned rendering must not invalidate current ownership.
