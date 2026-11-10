---
id: "01-preserve-current-watch"
title: "Preserve current GitLab watch opening"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.20
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.21
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.22
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.23
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
---

# Task 01: Preserve current GitLab watch opening

## Summary

Fence shared watch dialog UI state by its mounted opening. Keep original
requests and parent result publication authoritative after close/reopen.

## In scope

Real parent/domain/API/store regression, shared local lifetime repair,
review/issue create/update controls, disposal/scope/hydration, desktop/phone
transport proof, public GitLab watch behavior clarification and exact checks.

## Out of scope

Other integrations, MR linking, watch-list ownership and backend/API changes.

## Acceptance

- AC .20-.21: old completion publishes the captured request outcome and never
  changes the replacement draft/focus/busy/inline error or dismisses it.
- AC .22: current success closes; current error retains draft and permits retry;
  replacement Save/Cancel remain available, including while an old Save waits.
- AC .23: initial/default hydration, same-ID refresh, scope changes, unmount and
  StrictMode retain correct ownership. Browser cleanup preserves all checkout
  HEAD/full raw porcelain baselines and removes only owned fixtures.

## ASCII UI preview

UI-01: GitLab settings, newer Edit/Create opening after a pending Save is closed.

```text
Desktop                         Phone (same focused edit flow)
[Edit watch                 X]  [Edit watch                 X]
[Match fields]                  [Match fields              ]
[Workflow] [Step]               [Workflow                  ]
[Repository] [Profiles]         [Step                      ]
[Prompt / polling / cleanup]    [Repository / profiles     ]
[Cancel]        [Update watch]   [Prompt / polling / cleanup]
                                [Update watch              ]
                                [Cancel                    ]
```

Before: an old ACK unexpectedly dismisses the newer opening.
After: that ACK updates its original watch and feedback while this opening
retains raw draft/focus and owns its own Save/error. Its current successful
Save dismisses; current failure retains draft and permits retry.
AC .20-.23 require state ownership. Existing structure is retained; spacing
is illustrative. One dialog scroll owner, bounded by the dynamic viewport;
phone fields/actions stay stacked and desktop paired fields remain paired.

[Full preview and ordering](plan.md#ascii-ui-preview).

## Verification

Use Node 24 on PATH. Run from the repository root after the fresh `apps/` frozen install.

```bash
(cd apps/web && pnpm exec vitest run components/gitlab/watch-dialog-ownership.test.tsx components/gitlab/review-watch-dialog.test.tsx components/gitlab/issue-watch-dialog.test.tsx components/gitlab/watch-form.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/gitlab/watch-dialog.tsx components/gitlab/watch-dialog-ownership.test.tsx components/gitlab/watch-dialog-ownership.test-helpers.tsx e2e/tests/gitlab/*watch-dialog-ownership*)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 NODE_OPTIONS=--max-old-space-size=8192 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --project chromium e2e/tests/gitlab/watch-dialog-ownership.spec.ts -- --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 NODE_OPTIONS=--max-old-space-size=8192 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/gitlab/mobile-watch-dialog-ownership.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/components/gitlab/watch-dialog.tsx`
- `apps/web/components/gitlab/watch-dialog-ownership.test.tsx`
- `apps/web/components/gitlab/watch-dialog-ownership.test-helpers.tsx`
- `apps/web/e2e/tests/gitlab/{mobile-,}watch-dialog-ownership.spec.ts`
- `apps/web/e2e/tests/gitlab/watch-dialog-ownership-flow.ts`
- `apps/web/e2e/tests/gitlab/watch-dialog-ownership-fixture.ts`
- Existing GitLab Integrations requirement/design and `docs/public/integrations.md`.

## Dependencies

None. Parent immutable source proof is complete; author permanent tests before
production changes and retain actual failing behavioral assertion evidence.

## Risks

See plan: hydration, StrictMode, accurate old feedback and newer busy state.

## Parallelism

`sequential`; no subagents or additional sessions.

## Inputs

Existing requirement/design, shared dialog/wrappers/settings, actual GitLab
domain/API/store and nearest shipped browser fixtures. Parent source packet is
read-only diagnosis evidence; permanent regressions are independently authored.

## Results

- Frozen install from `apps/`: passed with pnpm 9.15.9 and Node 24.
- Causal RED before production: 2 intended old-ACK failures (review/issue) and
  2 ordinary owned-success controls passed. The first wider run also retained
  create-fixture URL errors separately from behavioral RED.
- Final focused Vitest command above: 4 files, 48 tests passed (38 new ownership
  cases plus 10 existing dialog/form controls). Uses the actual settings parent,
  domain hooks, API, fetchJson, store and providers; HTTP settlement and the
  unrelated Monaco renderer are the only stubs. Includes all AC .20-.23,
  StrictMode, abandoned render, hydration, raw focus/caret, newer busy state,
  original accepted publication/feedback, current failure/retry and Cancel.
- Targeted ESLint: passed without warnings. Web typecheck passed with 8 GiB;
  the earlier default 4 GiB heap exhaustion and test typing corrections are
  retained as setup/static failures, not causal RED.
- `i18n:check` and `i18n:ratchet`: passed; no copy/catalog changes. Existing
  orphan-key advisory remains. Final staged guards also run during normal commit.
- Managed fresh backend/Vite/plugin build: passed. Final desktop run: 3 passed
  in 11.8s; final mobile-chrome run: 2 passed in 11.3s. Both use one shard/worker,
  retries zero, real backend/mock GitLab and native phone `.tap()` interactions.
  Later test-only corrections reuse this unchanged production build with
  `--no-build`. The first browser run's wait-helper type error and missing
  explicit agent profile are retained as setup failures.
- Browser evidence covers review update and issue create, pending Cancel/reopen,
  original backend persistence, newer raw draft/focus/caret, successful next Save
  and reload. The desktop deliberate-primary-failure control passed; fixture
  teardown separately aggregates primary/cleanup errors, explicitly deletes its
  Worktree profile, removes owned resources, and verifies worker seed, owned
  source and task checkout HEAD/full raw porcelain before and after cleanup.
- Catalog: 397 decisions and 1712 specifications validated. Full specification
  lint passed; specification-linter tests: 36 passed. Public validators: 63 tests
  passed and 54 published pages validated. Public GitLab watch guidance updated
  in `docs/public/integrations.md` (existing how-to/reference page); no other
  provider section changed.
- PR documentation coverage `validateCoverage` preflight: covered, zero errors,
  using changed paths and the plan, work order, requirement and design contents.
  No numeric code-coverage instrumentation or unrelated broad suites were added.
- Bounded duplicate refresh reads all actual paginated files for PRs #1512,
  #3143, #4444 and #4517 plus any newly opened/moved PR since parent proof.
  The 69-open inventory and four heads stayed unchanged; #1512 watch patches
  concern Query/cache ownership, while other target sets are empty.
- Specification and diff gates pass. Raw logs, hashes, pre-TDD timestamps,
  setup failures and exact process/fixture receipts are retained externally;
  no real provider, physical device, operator instance or incident was exercised.
