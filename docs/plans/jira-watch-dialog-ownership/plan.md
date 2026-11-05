---
created: 2026-10-22
status: implemented
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCH-DIALOG-001
system_design:
  - ../../specs/integrations/system-design/jira-watch-dialog-lifetime.md
legacy_specs: []
---

# Implementation Plan: Jira Watch Dialog Ownership

## Overview

Preserve the current Jira watch form while previously accepted saves complete.
One sequential vertical work order owns implementation and validation. The user
explicitly authorizes package plus TDD in this session; publication remains
subject to a separate parent validation and delivery authorization.

## Scope

Only Jira watch dialog completion/dismiss/busy, necessary local helpers/tests,
the paired Integrations specs and this plan. Public docs may change only the
Jira watch paragraph in `docs/public/integrations.md`. Exclude connection
settings/health/read publication, Linear sibling work, other integrations,
polling/cache/API/backend/auth/schema, shared frameworks and global revisions.

## Technical approach

The immutable source at `80676b7f38ea53ed929de978b5770dd88b4a7e9e`
awaits a correctly captured write then unconditionally dismisses and clears
saving. The supplied actual-parent diagnostic fails once after accepted
store/table publication and a truthful toast; its ordinary owned control passes.
See the paired [design](../../specs/integrations/system-design/jira-watch-dialog-lifetime.md)
and [requirements](../../specs/integrations/requirements/jira-watch-dialog-lifetime.md).
Use one local mounted opening plus request token, semantic selection identity,
and explicit workspace-change invalidation. Preserve raw payloads and caller
publication. No active-workspace key for the install-wide parent.

## ASCII UI preview

UI-01: Jira settings, a later opening after an earlier save was accepted.
Shared desktop/phone control order; existing dialog owns vertical scrolling.

```text
Before old ACK:                 After old ACK (required):
[Edit/Create Jira watcher]      [Edit/Create Jira watcher]
[Workspace]                    [Workspace: current selection]
[JQL: later raw draft |]        [JQL: later raw draft |]
[Workflow] [Step]               [Workflow] [Step]
[Repository / branch]           [Repository / branch]
[Profiles / prompt / settings]  [Profiles / prompt / settings]
[Cancel] [Update/Create]        [Cancel] [Update/Create]
```

Current source instead dismisses the later dialog on old success. On phone,
retain the shipped single dialog scroll owner and reachable footer; use native
taps, not hover. Fields, focus and selection remain stable. A newer pending Save
keeps its own disabled Saving action. Spacing is illustrative; this package
changes ownership, not layout. Covers criteria .1, .3, .5 and .6.

## Tests

`components/jira/jira-watch-dialog-lifetime.test.tsx` owns the causal
reopen/update/create success/failure matrix, publication/toasts/raw payloads and
current retry/busy/Cancel (.1-.3), plus actual domain consumer hydration,
selection, scope, unmount/StrictMode/Suspense and workspace controls (.4-.5).
Retain existing dialog/section tests for independent prompt/default contracts.

## E2E tests

`jira-watch-dialog-ownership.spec.ts` and
`mobile-jira-watch-dialog-ownership.spec.ts` under
`apps/web/e2e/tests/integrations/` prove .1-.3 and .6 with a fresh shipped build,
actual settings route, disposable backend/mock Jira and after-persistence held
responses. Own unique workspace/repository/offline origin/workflow/step/profile
with explicit Worktree executor. Compare full task code and worker seed before,
during and after; delete only owned records/roots, including profile. Aggregate
primary and cleanup errors and run a deliberate failure cleanup control.

## Work orders

- [x] [Task 01: Preserve the current watch](task-01-preserve-current-watch.md)

## Verification strategy

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/jira/jira-watch-dialog-lifetime.test.tsx components/jira/jira-issue-watch-dialog.test.tsx components/jira/jira-issue-watchers-section.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium e2e/tests/integrations/jira-watch-dialog-ownership.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome e2e/tests/integrations/mobile-jira-watch-dialog-ownership.spec.ts --retries=0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/jira/jira-issue-watch-dialog.tsx components/jira/jira-watch-dialog-lifetime*.tsx e2e/tests/integrations/*jira-watch-dialog* --max-warnings=0)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run Go builds with `GOMAXPROCS=2 GOFLAGS=-p=2`; use Node 24 from
`/home/jcfs/.nvm/versions/node/v24.18.0/bin`. Preserve every long command handle.
Run repository documentation coverage preflight against the complete changed
file list and confirm all plan references before commit. Normal active pre-commit
and commit-msg hooks are required; no bypass/amend. No suite overlap or relaxed
worker, retry, memory or WebSocket guards.

## Verification results

- Immutable-source causal replay at `80676b7f38ea53ed929de978b5770dd88b4a7e9e`:
  one intended missing-dialog failure after real store/table publication and
  success toast, plus one passing ordinary owned-save control. No setup/import
  or unhandled errors in this behavioral RED (9.72 s total, 835 ms tests).
- Final focused Vitest command above: **3 files, 33 tests passed** (25.70 s).
  This includes actual parent/domain/API/store/providers; only unrelated Monaco
  rendering is adapted. Additional real-domain consumers cover prop lifetimes.
- Fresh managed desktop command above: **3 passed**, 37.9 s browser execution.
  Fresh managed Pixel 5 command above: **2 passed**, 28.1 s browser execution.
  One shard/worker, retries zero, normal resource and WebSocket guards. Browser
  cases use actual settings routes, native phone taps and held responses after
  real disposable backend persistence. Raw frontend JQL remains verbatim;
  persistence assertions retain the existing backend trimming.
- All final fixture cleanup receipts have no cleanup errors; the deliberate
  flow failure propagates its original error after verified cleanup. Unique
  workspace/repository/offline origin and Worktree profile are owned by the
  fixture. Profile deletion and workspace removal are queried. Complete task
  source, worker seed and owned source bytes, HEAD and full raw porcelain stay
  unchanged before/during/after flow and API cleanup. Fixture and backend roots
  are absent after runner teardown. Earlier ENOBUFS setup roots were verified
  empty and removed by exact path; all are recorded externally.
- `pnpm install --frozen-lockfile` from `apps/`: passed. Final 8 GiB Node 24
  typecheck, strict changed-file ESLint, i18n check/ratchet: passed. No new copy
  or locale keys. Catalog validation, specification linter tests, full spec
  lint, public validators and documentation coverage preflight: passed.
- Full Playwright discovery across chromium/mobile-chrome/containers: no
  discovery errors. No full suite or container/provider/device claim.
- Public documentation impact: reference/how-to paragraph added only to Jira
  issue watches in `docs/public/integrations.md`; other provider and connection
  paragraphs retained. Linear sibling shared-path scope was coordinated.
- Duplicate refresh: 69 open PRs. Actual paginated targets read at unchanged
  heads for 1512 (895 files, 9 pages), 4375 (19), 4517 (238) and 3440 (180).
  PR 1512 changes data sources/cache and retains unconditional await-close/finally
  in its full dialog source; the other three have no Jira watch targets.
  This is bounded inspected-target evidence, not an issue-backlog assertion.
- Tool/setup corrections were kept separate from behavioral evidence: initial
  pnpm invocation directory, copy path, fixture output buffer, nullable empty
  lists, API create workspace location, JQL message wording and persisted
  trimming. Source guard never repairs or resets a checkout.
- External evidence: `/home/jcfs/.kandev/tasks/preserve-newer-jira_lqkk6w2w/jira-watch-evidence/`.
  Pre-TDD package timestamps/hashes precede permanent tests. Normal active
  pre-commit/commit-msg hook receipt, exact committed clean HEAD, complete
  binary/full-index diff/hash and parent rerun command are captured there at
  the parent-validation checkpoint. No push/PR/merge is authorized.


## Risks

- Treating passive global workspace changes as form replacement loses drafts.
- Resetting all create fields on workspace selection violates raw data continuity.
- Guarding domain actions would lose accepted publication/toasts.
- Render-time ref invalidation would retire a still-visible suspended owner.

## Delivery boundary

First checkpoint is `ready_for_parent_validation`: normal-hook committed clean
HEAD, all owned handles terminal, fixtures/resources removed. External packet
must include task/session/base/full HEAD, all absolute changed paths, complete
binary/full-index base-to-head diff plus SHA256, full source/docs and receipts.
Parent independently reads the full diff and reruns focused tests before a
separate same-session delivery authorization. No push, PR or merge before that.

## Review remediation

The browser fixture now compares the source snapshots stored in each checkpoint
receipt instead of taking a second observation. This keeps the compared state
and recorded state identical and removes duplicate repository scans. Production
form behavior, requirements, design and public copy are unchanged.

An external diagnostic executes the actual fixture check function with controlled
snapshot reads. The original check missed a captured mutation when the later read
was restored (one intended RED, ordinary stable-source control passed). The
corrected check passes both controls. This is fixture-boundary evidence, separate
from the real consumer/HTTP and shipped-browser proofs above; it adds no product
export or permanent source-text assertion.

Post-remediation local commands passed: the two-control diagnostic, strict ESLint
for the changed fixture, the normal 8 GiB typecheck, and the documented managed
Jira browser commands (desktop 3 passed in 29.8 s; Pixel 5 native taps 2 passed in
35.1 s), with zero retries and normal guards. All five fixture cleanup receipts
have no cleanup errors and preserve source/seed/owned-repository state. Existing
permanent browser controls remain the fixture's runtime coverage.

Exact committed-head capture, CI, semantic review and combined-base delivery
receipts are tracked externally and refreshed after the remediation commit;
prior delivery evidence remains historical rather than a claim about a new head.
