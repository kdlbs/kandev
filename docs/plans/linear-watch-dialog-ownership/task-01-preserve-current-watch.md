---
id: "01-preserve-current-watch"
title: "Preserve current Linear watch form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-LINEAR-WATCH-DIALOG-001
acceptance_criteria:
  - AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.1
  - AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.2
  - AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.3
  - AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.4
  - AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.5
  - AC-INTEGRATIONS-LINEAR-WATCH-DIALOG-001.6
system_design:
  - ../../specs/integrations/system-design/linear-watch-dialog-lifetime.md
---

# Task 01: Preserve current watch

## Summary

Keep the current form intact while an older accepted save settles. Preserve
original writes, real store/table publication and truthful feedback.

## In scope

Dialog opening/submission ownership and faithful consumer/transport/browser tests.

## Out of scope

Connection settings, other providers, polling/cache authority and backend/API.

## Acceptance

- All six linked criteria have mounted consumer evidence through real actions.
- Fresh desktop and phone production-browser checks prove the shipped flow.
- Clean normal-hook commit and external full packet; no push or PR.

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

See [combined preview](plan.md#ascii-ui-preview).

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

## Files likely touched

- `apps/web/components/linear/linear-issue-watch-dialog.tsx`
- `apps/web/components/linear/linear-watch-dialog-lifetime.test.tsx`
- `apps/web/components/linear/linear-watch-dialog-lifetime.test-helpers.tsx`
- `apps/web/e2e/tests/integrations/linear-watch-dialog-{fixture,flow,test,ownership.spec}.ts`
- `apps/web/e2e/tests/integrations/mobile-linear-watch-dialog-ownership.spec.ts`
- `docs/public/integrations.md` only if its Linear watch paragraph needs clarification.

## Dependencies

None.

## Risks

Editable create workspace scope differs from workspace-keyed Sentry settings.

## Parallelism

`sequential`

## Inputs

Paired requirement/design and external attempt32-source proof.

## Results

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
