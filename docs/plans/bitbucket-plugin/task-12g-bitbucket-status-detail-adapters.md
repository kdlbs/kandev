---
id: "12g-bitbucket-status-detail-adapters"
title: "Adapt Bitbucket to shared task status and review detail"
status: completed
wave: 3g
depends_on: ["12e-shared-task-status", "12f-shared-review-detail"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
decision: "../../decisions/2026-08-06-plugin-code-host-dashboard-parity.md"
---

# Task 12g: Adapt Bitbucket to shared task status and review detail

## Intent

Remove the plugin-owned status slot and bespoke tabbed Review panel. Feed Bitbucket
Cloud/Data Center data and supported actions into the exact host surfaces from tasks
12e and 12f, then prove live pipeline/link/review behavior.

## Owned paths

- `kdlbs/kandev-plugin-bitbucket/ui/src/{bundle,task-review-status,view-models}.ts`
- `kdlbs/kandev-plugin-bitbucket/ui/{plugin.css,test/**,e2e/**}`
- Bitbucket adapter/domain fixtures needed for normalized detail/status fields
- packaged plugin bundle and manifest version

## Implementation

1. Hydrate review-provider snapshots with head-commit build statuses and normalized
   shared `taskStatus`; remove the `chat-top-bar` visual registration and its poller.
2. Map Bitbucket identity/state, author/timestamps, branches/stats, description,
   participants/reviews, checks, threads, and supported mutations into
   `host.ui.ChangeRequestDetail`.
3. Remove custom Review tabs/actions/CSS. Declare approve/unapprove/merge/decline,
   comment/reply, refresh, and external links only when Cloud/Data Center capabilities
   and viewer state permit them.
4. Preserve durable manual, auto-discovered, created, and watch-owned PR association as
   the single source for review/status queries.
5. Rebuild/package a fresh version and restart the evaluation instance with exact served
   asset hashes.

## TDD and acceptance

1. RED plugin tests require status in the registry snapshot, no visual slot, and exact
   normalized detail/action mapping; cover links objects without string coercion.
2. RED desktop/mobile E2E requires topbar popover, composer CI chip, shared Review
   sections, action dispatch, provider links, no custom tabs, and no overflow.
3. Live Cloud acceptance changes the source-commit pipeline
   `SUCCESSFUL -> INPROGRESS -> FAILED -> SUCCESSFUL`, verifies both status surfaces
   update, opens Review from each, and restores the fixture.
4. Verify manual linking and exact-branch auto-link survive restart and feed both status
   surfaces. Data Center remains fixture-covered until a disposable target exists.
5. Run plugin unit/race/vet/build/package checks plus focused host E2E.

## Risks

- Bitbucket Cloud and Data Center expose different build/review detail shapes; normalize
  capabilities without pretending unsupported operations exist.
- Live acceptance must restore the disposable pipeline state even after a failed test.
- Same-version bundle caching requires a version bump and fresh document.

## Completed verification (2026-08-06)

- Plugin UI tests passed (43), all Go packages passed, `go vet ./...` passed, and the
  host-only `0.1.10` package verified and installed active in the evaluation instance.
- Live Cloud commit status completed the required
  `SUCCESSFUL -> INPROGRESS -> FAILED -> SUCCESSFUL` cycle; both shared status surfaces
  reflected each state and the fixture was restored.
- Manual Link dialog submission persisted one association. Exact-branch auto-link was
  proved by unlinking the disposable task, observing zero associations, loading the task,
  and observing the matching PR association recreated.
- Cloud review detail exposed real PR data/actions through the shared host component;
  Cloud and Data Center timestamp fixtures cover provider normalization, while a live
  Data Center target remains intentionally out of scope for this acceptance run.
