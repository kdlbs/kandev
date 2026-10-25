---
created: 2026-10-22
status: done
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
legacy_specs: []
---

# Implementation Plan: GitLab watch dialog ownership

## Overview

Preserve the current opening when an older GitLab watch Save settles. Parent
source-first proof on base `259e62b85f1768c506d1358d8215a0a6d6be1fdc`
confirmed that unconditional dialog dismissal after await closes a later draft;
ordinary owned success passes. The same shared form resets on watch-object
hydration and retains busy state across closure. One sequential TDD pass owns
all changes. The user explicitly authorizes design followed by implementation
without another planning approval; publication is excluded until parent validation.

## Scope

In scope: shared review/issue create/update form lifetime, local settlement,
real transport/domain/store regressions, disposable desktop/phone verification.
Out of scope: GitHub connection, TaskMRLinkDialog, Linear settings, backend,
watch-list authority, Query/polling migrations, persistent drafts and new APIs.

## Technical approach

Use the existing Integrations vertical pair (AC .20-.23). A conditionally
mounted, identity-keyed local form and committed liveness fence isolate UI
settlement. Preserve domain/API callbacks and normalized payloads. No ADR or
new framework: the design section records the local tradeoff completely.

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

## Tests

`components/gitlab/watch-dialog-ownership.test.tsx` owns real parent/domain/API/
store integration evidence for AC .20-.23; existing form tests retain payload
and initial/default controls. Both kinds and create/update share the same owner.

## E2E tests

`e2e/tests/gitlab/watch-dialog-ownership.spec.ts` (chromium) and
`mobile-watch-dialog-ownership.spec.ts` (mobile-chrome) exercise the shipped
settings, pending Cancel/reopen, draft/focus, original persistence and next Save.
Use one worker/shard, retries zero, owned disposable data and guarded checkout
cleanup. Only the causal transport settlement is delayed.

## Work orders

- [x] [Task 01: Preserve current watch](task-01-preserve-current-watch.md)

## Verification

Use Node 24 on PATH. Fresh workspace dependency install from `apps/` precedes all pnpm commands.
All paths below are repository-relative; execute commands from the root.

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

## Verification results

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

## Risks

Same-ID hydration must not invalidate pending ownership or erase a raw draft.
StrictMode replay must reactivate the same form; scope changes and reopening
must retire it. Old rejection remains truthful in parent feedback but must not
become the replacement form's inline error. Never suppress accepted store writes.
