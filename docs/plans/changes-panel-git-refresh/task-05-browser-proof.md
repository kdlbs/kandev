---
id: "05-browser-proof"
title: "Prove integrated recovery and document behavior"
status: done
wave: 5
depends_on: ["04-status-ui"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.19
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.22
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.23
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.24
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.30
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.32
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 05: Prove integrated recovery and document behavior

## Summary

Prove the delivered behavior on isolated desktop and phone instances. Reconcile the accepted documentation only after all work orders satisfy the requirement.

## In scope

- Complete the focused E2E suites introduced red in Task 04 using disposable repository/backend fixtures and causal waits.
- Gate real enrichment, drop initial status notifications, expire a slow basic request, and recover without another file edit. Keep private source workspaces out of fixtures.
- Cover clean/loading/error/prior-good, partial repository failure, shared siblings, pending layer selection, reconnect, and replacement.
- Inspect rendered phone/desktop evidence against UI-01/UI-02 and preserve scroll, touch, dismissal, and safe-area behavior.
- Update public Changes recovery guidance, source-precedence prose, scoped engineering guidance if changed, and all statuses/results.
- Promote the requirement/design and mark the plan implemented after conformance and listed checks pass.

## Out of scope

Unrelated suites, executor provisioning, commits, push, and PR creation.

## Acceptance

- Both browser projects prove file rows before enrichment and recovery after a missed snapshot without Git mutation. Pending selected diffs remain open.
- Mobile loading/error/Retry behavior matches desktop capability and passes touch/scroll/navigation checks. Both screenshots match the structural previews.
- Every criterion has recorded evidence, all permanent changes are covered, and public/spec/plan guidance describes the final implementation.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-01: Changes status (desktop)

Entry: task Changes panel. The existing toolbar remains above the single scroll body.

```text
Changes                                      [Refresh]
Loading:       Checking changed files...
Unavailable:   Git status unavailable.        [Retry]
Prior data:    Refresh failed. Showing last observed changes. [Retry]
Dirty:         Changed files are ready. Diffs are loading.
  Unstaged
    src/a.ts   Modified                       Diff pending
  Staged
    src/b.ts   Modified                       Diff pending
Clean:         Your changed files will appear here
```

### UI-02: Changes status (phone)

Entry: task bottom navigation > Changes. Reuse the shipped mobile Changes body and full-height diff drawer.

```text
< Task                 Changes
Git status unavailable.
[ Retry (touch target) ]
Unstaged
  src/a.ts  Modified
  Diff pending
-----------------------------
[Sessions] [Files] [Terminal] [Changes]

< Back        src/a.ts
Diff is loading...
```

The status row scrolls with the existing body. Phone navigation and drawer controls retain their safe-area behavior.
Retry is 44px on touch surfaces and 28px on desktop. Each surface has one vertical scroll owner.
The clean line appears only after complete successful empty membership, with no independent PR or commit content.
Row selection opens a pending placeholder and remains open until authoritative membership removes that repository/path/layer.
These structures satisfy criteria `.23` through `.30`. Copy and spacing are illustrative and must use localization and existing primitives.

## Verification

Run from repository root. Retain Task 04 red evidence for rendered changes. Use controlled integration cases to prove the completed producer/delivery contract.
If `apps/node_modules` is absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-panel-refresh-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-changes-panel-refresh-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

## Files likely touched

- `apps/web/e2e/tests/git/changes-panel-refresh-recovery.spec.ts`
- `apps/web/e2e/tests/git/mobile-changes-panel-refresh-recovery.spec.ts`
- `apps/web/e2e/fixtures/git-shim.mjs`
- `apps/web/e2e/helpers/git-helper.ts`
- `apps/web/e2e/helpers/causal-waits.ts`
- `docs/public/sessions-and-review.md`
- `apps/backend/internal/agentctl/AGENTS.md`
- `apps/web/AGENTS.md`
- `docs/specs/platform/requirements/workspace-git-status.md`
- `docs/specs/platform/system-design/workspace-git-status.md`
- `docs/specs/tasks/system-design/environment-owned-git-status.md`
- `docs/decisions/2026-09-30-progressive-workspace-git-refresh.md`
- `docs/plans/changes-panel-git-refresh/plan.md`

## Dependencies

Complete Task 04 first. Read its Results before changing the shared contract.

## Risks

- Browser fixtures must seed an isolated instance, never the developer database. Teardown only owned processes.
- Store injection alone cannot prove tracker delivery. Use it for presentation cases and retain a real Git/HTTP/stream recovery scenario.
- Keep one worker per managed shard and run desktop/mobile commands sequentially. Record actual discovered test counts.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md).
- [System design](../../specs/platform/system-design/workspace-git-status.md).
- [Plan](plan.md), accepted ADR, scoped `AGENTS.md`, and existing tests beside owned code.

## Results

Desktop and mobile recovery E2E passed after red-driven coverage exposed the shared mobile drawer's 80vh cap. The first full-height assertion measured 581.6 CSS px in a 727 px viewport; `MobileDiffSheet` now uses dynamic viewport height, and the final test asserts at least 95% viewport height with its top edge inside 5% of the viewport. Both recovery flows use isolated browser fixtures and recover through the correlated response without a Git mutation. The mobile test also checks touch Retry, selected pending diff, and horizontal overflow.

- `(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-panel-refresh-recovery.spec.ts)`: passed (1 test).
- `(cd apps/web && pnpm e2e:run --capture --project mobile-chrome tests/git/mobile-changes-panel-refresh-recovery.spec.ts)`: passed (1 test), including full-height drawer bounds.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -g "tapping a staged file row opens file diff sheet")`: passed (1 test).
- Desktop and mobile capture-mode screenshots were inspected against the design. The E2E runner built the backend, Vite assets, and fixture plugin; generated `apps/web/.pr-assets` output was removed.
- Frontend tests passed (19 files, 188 tests); `pnpm run typecheck`, web lint, `i18n:zh-hant`, `i18n:pseudo`, `i18n:check`, and `i18n:ratchet` passed.
- `python3 scripts/list-docs.py validate`: passed (334 decisions, 1262 specifications); `python3 scripts/lint-spec-files.py --all`: passed; `node --test scripts/validate-public-docs.test.mjs`: passed (62 tests); `node scripts/validate-public-docs.mjs`: passed (47 pages).
- Accepted the Platform requirement/design and publication ADR; reconciled Tasks source-precedence prose and public recovery guidance. Final `git diff --check` passed.
