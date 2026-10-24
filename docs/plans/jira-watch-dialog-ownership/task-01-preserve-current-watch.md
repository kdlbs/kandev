---
id: "01-preserve-current-watch"
title: "Preserve the current Jira watch form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCH-DIALOG-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.1
  - AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.2
  - AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.3
  - AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.4
  - AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.5
  - AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.6
system_design:
  - ../../specs/integrations/system-design/jira-watch-dialog-lifetime.md
---

# Task 01: Preserve the Current Watch

## Summary

Implement the local opening/request ownership boundary from the paired design.
First preserve the immutable causal RED and ordinary control, then add the real
consumer matrix and the smallest GREEN correction.

## In scope

Jira watch dialog completion/dismiss/busy and necessary local tests/helpers;
workspace picker/default lifetimes, original transport/publication/toasts,
desktop and Pixel 5 shipped browser proof, owned cleanup, spec/docs and hooks.

## Out of scope

Connection settings/health/shared coordinators; Linear/Sentry/GitLab and all
polling/cache/API/backend/auth/schema/credential/server-ordering changes. No
shared framework, ADR, global queue or durable authority.

## Acceptance

1. Real parent/domain/API/store/provider regressions cover all listed criteria,
   retain original raw JQL/captured target and truthful feedback, and fail for
   the intended causal defect before the local correction.
2. Current and replacement forms retain correct busy, retry, Cancel, focus,
   selection and explicit versus passive workspace semantics.
3. Fresh desktop/mobile browser controls and cleanup/failure controls pass;
   static/spec/public checks and active normal hooks produce a clean committed
   checkpoint with complete external evidence and no publication.

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


Full preview and contract mapping: [plan](plan.md#ascii-ui-preview).

## Verification

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

## Files likely touched

- `apps/web/components/jira/jira-issue-watch-dialog.tsx`
- `apps/web/components/jira/jira-watch-dialog-lifetime.test.tsx`
- `apps/web/components/jira/jira-watch-dialog-lifetime.test-helpers.tsx`
- `apps/web/e2e/tests/integrations/jira-watch-dialog-ownership*.ts`
- `apps/web/e2e/tests/integrations/mobile-jira-watch-dialog-ownership.spec.ts`
- `docs/public/integrations.md` (Jira watch paragraph only)
- Paired Integrations requirements/design and this plan/work order.

## Dependencies

None. Source proof and duplicate patch inventory are external inputs.

## Risks

See [plan](plan.md#risks). Assert real HTTP/store/provider behavior rather than
mocking admission/actions; disclose only the unrelated Monaco renderer adapter.

## Parallelism

`sequential`; no delegation authorized.

## Inputs

Paired requirement/design, actual install-wide caller, immutable proof and
source receipts in `attempt33-source`, shipped Sentry opening pattern and
owned E2E fixture conventions. User has settled scope and workspace semantics;
no material interview questions remain.

## Results

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
