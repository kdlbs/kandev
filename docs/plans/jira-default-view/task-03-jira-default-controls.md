---
id: 03-jira-default-controls
title: Add picker controls and prove the flow
status: done
wave: 3
depends_on:
  - 02-restore-jira-default
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.1
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.2
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.4
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.5
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.6
  - AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.7
system_design:
  - ../../specs/integrations/system-design/jira-default-view.md
---

# Task 03: Add picker controls and prove the flow

## Summary

Add a default marker and accessible set/clear action to every Jira view row, then verify the behavior in desktop and phone browsers. Update the public Jira guide.

## Scope and likely files

- `apps/web/components/jira/my-jira/list-toolbar.tsx` and `list-toolbar.test.tsx`
- `apps/web/app/jira/jira-page-client.tsx` for action props and user-visible failure feedback
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/jira.json`; use `pnpm run i18n:zh-hant` for the Traditional Chinese pair
- `apps/web/e2e/tests/integrations/jira-default-view.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-jira-default-view.spec.ts`
- `docs/public/integrations.md`

## Exclusions

- No new phone navigation surface and no other provider's picker changes.

## Implementation acceptance

1. Both built-in and custom rows show their default state; set/clear actions leave the current view and delete action untouched, including while a write is pending or fails.
2. Desktop and phone browser flows prove a custom JQL default on revisit and fallback after deletion; phone actions have at least 44px touch targets and no document horizontal overflow.
3. All new copy is localized in the required catalogs, and the public Jira guide explains how to choose or clear a default.

## ASCII UI preview

`UI-01`, excerpt from the [combined plan preview](plan.md#ascii-ui-preview). The star is a sibling action, not part of the select button.

```text
Desktop: [check] Assigned to me       [Set default]
                 My open tickets [star: default] [Delete]

Phone:   [check] Assigned to me   [star, 44px]
                 My open tickets  [star, 44px] [Delete, 44px]
```

The picker remains the entry point and its list owns scrolling. The marker is visible; its localized accessible name distinguishes set from clear. This covers `AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.6`.

## TDD and verification

Add a toolbar regression for action isolation and accessibility and browser scenarios for desktop and phone. Confirm expected red failures, implement, then run:

```bash
cd apps/web && pnpm exec vitest run components/jira/my-jira/list-toolbar.test.tsx
cd apps/web && pnpm run i18n:check
make build-web
cd apps/web && pnpm e2e:run tests/integrations/jira-default-view.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-jira-default-view.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run `make build-web` and both documentation commands from the repository root. Record exact red and green results here.

## Dependencies and risks

Depends on Task 02's mutation and selection API. The current picker is narrow; verify actual touch hit boxes and viewport containment in the phone scenario. A pending default update must not allow a conflicting delete or star action.

## Result

The Jira saved-view picker now marks the active default and provides a sibling set/clear action for built-in and custom views. The action has a localized accessible name without a duplicate toggle-state announcement, does not select or delete its row, and stays at least 48px square on phones. The active default star remains visible on fine-pointer desktop. The picker owns vertical scrolling. Default writes publish only after persistence succeeds; pending writes disable save, star, and delete controls. Save and delete failures show localized retryable feedback. Jira project discovery no longer blocks the page while the initial view resolves, and status reconciliation prunes saved statuses only after authoritative metadata arrives for the current workspace and project set.

The public Jira guide now explains how to set or clear the default. The required Jira copy is present in English, Portuguese, Simplified Chinese, both generated Traditional Chinese catalogs, Japanese, and the pseudo-locale.

Red: the toolbar tests failed because the default buttons were absent. The desktop browser scenario timed out waiting for the missing set-default action. A new first-render status-readiness regression test failed because an effect observed the previous key set as loaded.

Green:

```text
pnpm exec vitest run components/jira/my-jira/use-project-statuses.test.ts components/jira/my-jira/jira-default-view.test.ts components/jira/my-jira/use-saved-views.test.ts app/jira/jira-page-client.test.tsx components/jira/my-jira/use-jira-search.test.ts components/jira/my-jira/list-toolbar.test.tsx
6 files passed, 33 tests passed
pnpm run typecheck
PASS
pnpm run i18n:check
PASS
pnpm e2e:run tests/integrations/jira-default-view.spec.ts
1 passed
pnpm e2e:run --project mobile-chrome tests/integrations/mobile-jira-default-view.spec.ts
1 passed
make build-web
PASS
go test ./internal/user/dto ./internal/user/service ./internal/user/store ./internal/settingscatalog ./internal/user/controller
PASS
node --test scripts/validate-public-docs.test.mjs
62 passed
node scripts/validate-public-docs.mjs
47 pages validated
git diff --check
PASS
```

The broad `pnpm run i18n:zh-hant` command was blocked by two pre-existing Simplified Chinese-looking workflow catalog entries. The Jira Traditional Chinese pair was generated successfully with `node scripts/convert-zh-cn-to-zh-hant.mjs --locale all --namespace jira --write`; the full i18n check passes.

## Review follow-up

Follow-up review fixes preserve saved structured statuses and custom JQL when project-status metadata is unavailable. They serialize saved-view list mutations, publish a new view only after its settings write succeeds, and keep the active default star visible on fine-pointer desktop. A newer selection or filter edit also wins over a pending save or deletion response. Save is disabled during settings mutations; rejected saves leave no local view to mark default and show localized failure feedback. The desktop and mobile browser specs use the causal HTTP wait helper for settings writes.

The focused regressions cover failed status lookups for saved queries, a deferred save or deletion followed by a newer selection, rejected saves followed by an attempt to set the missing ID as default, both pending-mutation states, active desktop-star visibility, and initial-search loading.

```text
pnpm exec vitest run components/jira/my-jira/use-project-statuses.test.ts components/jira/my-jira/use-saved-views.test.ts components/jira/my-jira/jira-default-view.test.ts components/jira/my-jira/use-jira-search.test.ts components/jira/my-jira/list-toolbar.test.tsx app/jira/jira-page-client.test.tsx
6 files passed, 47 tests passed
pnpm run typecheck
PASS
pnpm exec eslint app/jira/jira-page-client.tsx app/jira/jira-page-client.test.tsx components/jira/my-jira/jira-default-view.ts components/jira/my-jira/jira-default-view.test.ts components/jira/my-jira/jira-default-view-action.tsx components/jira/my-jira/list-toolbar.test.tsx components/jira/my-jira/use-jira-filter-state.ts components/jira/my-jira/use-project-statuses.ts components/jira/my-jira/use-project-statuses.test.ts components/jira/my-jira/use-saved-views.ts e2e/tests/integrations/jira-default-view.spec.ts e2e/tests/integrations/mobile-jira-default-view.spec.ts
PASS
pnpm run i18n:check
PASS
pnpm run e2e:sleep-ratchet
PASS
pnpm e2e:run tests/integrations/jira-default-view.spec.ts
1 passed
pnpm e2e:run --project mobile-chrome tests/integrations/mobile-jira-default-view.spec.ts
1 passed
```

## CI retry follow-up

PR #3924 merged on 2026-09-25. Its E2E retry report identified 11 retry-only passes. Follow-up changes restore persisted model/config state when the live session cache is empty after restart; correlate the taskless RoutineRun with the live Office run through their shared causation ID; position the mobile PR selector above the bottom navigation; wait for its prior selection to close and its menu animation to finish; scope dropped WebSocket replies to the test session; and wait for screenshot and virtualized file-tree content before asserting. Mobile touch-size assertions use unrounded measured dimensions.

```text
chromium taskless-routine-session.spec.ts, repeat-each=10, retries=0
10 passed
mobile PR re-request-review, threads saved-view, and session-entry-recovery scenarios, retries=0
15 passed across repeat counts 3, 3, and 3 (two PR and two saved-view cases per repeat)
Quick Chat backend-restart recovery, repeat-each=3, retries=0
3 passed
completed-workspace restoration, repeat-each=2, retries=0
2 passed
preview feedback, repeat-each=3, retries=0
3 passed
pnpm run typecheck
PASS
focused ESLint on changed web files
PASS
go test ./internal/backendapp -run 'TestAppendSessionModelsMessage' -count=1
PASS
git diff --check
PASS
```

The live Office run-list response now includes its existing causation ID so the E2E can require an exact match with the RoutineRun returned by the manual fire. `TestRunToListItemPreservesCausationID` covers this response contract. The corrected PR head's CI will provide final shared-runner confirmation.

## PR #3936 CI retry remediation

The PR #3936 retry artifact reported nine retry-only E2E failures. The fixes add causal waits for cancellation, file-tree, reload readiness, and persisted responses; select the seeded repository by ID; open the created task directly for the terminal check; and tolerate subpixel representation at the 44px touch-target boundary. A backend regression reproduces worktree cleanup failing when a checkout disappears between the path check and `git rev-parse`; cleanup now uses the branch commit only after confirming that disappearance.

Focused no-retry repetitions pass for all changed desktop and mobile cases (10 desktop, 8 mobile). `go test ./internal/worktree ./internal/task/service ./internal/backendapp`, web typecheck, focused ESLint, E2E sleep ratchet, and `git diff --check` pass. The updated PR head's CI and retry artifact remain the final shared-runner verification.

## Latest PR #3936 CI follow-up

The exact-head blob audit reported ten retry-only scenarios and one file-tree test that failed all attempts. The follow-up removes the observed E2E races and fixture coupling: wait for file-tree and screenshot-capture readiness, open tasks by ID when cards update, isolate run-observation agent data, keep the workflow task command deterministic, restore PR-chip focus after a 2-to-1 unlink, retain a completed workflow preview during quick surface handoff, and confirm mobile prompt selection before checking its composer chip. No timeout was raised for the passthrough failure.

```text
pnpm run build:e2e
PASS
chromium markdown preview, pane isolation, run observation, preview feedback, and workflow lifecycle, repeat-each=3, retries=0
15 passed
chromium office error handling, repeat-each=10, retries=0, CI=true, GITHUB_ACTIONS=true, IPv4-first DNS
10 passed
mobile menu hierarchy and PR unlink, repeat-each=3, retries=0
6 passed
mobile passthrough composer prompt selection, repeat-each=10, retries=0
10 passed
chromium large file tree, repeat-each=3, retries=0
3 passed
mobile workflow move preview, repeat-each=5, retries=0
5 passed
PR chip and workflow preview focused unit tests
59 passed
web typecheck and ESLint on changed files
PASS
git diff --check
PASS
```

The original office `TypeError: fetch failed` did not recur in ten runs with CI environment settings. Exact-head CI and its zero-retry blob audit remain pending for the pushed follow-up.

## PR #3936 retry follow-up after e767f4d

The exact-head E2E artifact for `e767f4d698e0f88bbefdb6aa49b91ce14710088e` contained six retry-only scenarios. Fixes isolate workflow steps from mutable worker data; navigate to tasks by ID; wait for the source environment, fixture file, and causal file-tree response; restore and confirm the shared Office CEO is idle before each routine fire; and tolerate a worktree that disappears during Git status inspection. Office-specific reset/settings calls use the existing backend transport-recovery wrapper. Preview cleanup timers are drained in the fake-timer component test teardown.

```text
Four other Chromium retry scenarios, repeat-each=3, retries=0
12 passed
Taskless routine, repeat-each=3, retries=0
3 passed
Mobile changes panel, repeat-each=3, retries=0
3 passed
pnpm exec vitest run components/task/workflow-move-proceed-button.test.tsx
26 passed
pnpm run typecheck
PASS
Focused ESLint on changed web files
PASS
pnpm run e2e:sleep-ratchet
PASS
go test ./internal/worktree ./internal/task/service ./internal/backendapp
PASS
git diff --check
PASS
```

Kubernetes compatibility had failed on the previous PR head while downloading external tools after repeated HTTP 500 responses. Exact-head CI and a zero-retry blob audit remain pending for this fixup.

## PR #3936 retry follow-up after 4059bcef

The exact-head retry summary for `4059bcef607b7da9050de8cfd4c7e2a111999288` reported four retry-only scenarios and one compact-stepper test that failed all attempts. The compact-stepper fixture now creates enough workflow steps to select the compact layout at the tested viewport. Symlink and rename tests wait for the prepared environment, exact fixture files, and a fresh workspace-tree response. The slow Docker task now selects its custom executor profile before environment setup. Mobile file-comment selection uses a ref so dropdown close timing cannot lose the action.

Retry-free local checks pass: the workflow-stepper pair passed three repetitions each (6); directory and editor symlinks plus rename commit/cancel passed three repetitions each (15); the mobile file-comment flow passed five repetitions; and the Docker slow-bootstrap flow passed three repetitions. The review toolbar unit suite passes 12 tests. Web typecheck, focused ESLint, E2E sleep ratchet, and `git diff --check` pass. Exact-head CI and the explicit blob retry audit are pending for this fixup.

## PR #3936 exact-head E2E follow-up after 42886d6

The exact-head run for `42886d63340ab5823c8e1002d50f302e83292d93` reported four retry-only scenarios and two rename tests that failed every attempt. The rename setup helper had been converted to an options object at only two of its four call sites. The retry cases exposed fixture commits that were not pushed to the task's source branch, file-tree reads that raced worktree materialization, an import assertion that raced SPA navigation, and a PR-detection watcher attached after the existing WebSocket had opened. The rename fixtures now use the new helper shape and publish their seed commits on `main`; file-tree cases wait for exact worktree content and a fresh tree response; workflow import stays on its existing page; and PR detection subscribes before navigation and waits for the exact persisted PR update emitted by the poller.

The changed no-retry E2E cases pass three repetitions each (21 total): PR detection, chat context, file and directory download, blur and no-op rename, and workflow import. The first local PR-detection run exposed that `watchWs` must attach before navigation; after moving it, the PR case passed three repetitions alone and in the combined run. Web typecheck, focused ESLint, E2E sleep ratchet, E2E bundle build, and `git diff --check` pass. The next exact-head CI and blob audit will confirm these changes on the shared runner.

## PR #3936 E2E fixup after 3a6e83b

The exact-head blob audit for `3a6e83b1d6245bb42bff774874bb11786791d967` found three retry-only E2E flakes and one preview-session test that failed all attempts. The preview test now follows the persisted primary session and checks that session's response. The Office manager test waits for the browser's agent-list response and verifies the CEO and worker fixtures before opening the picker. Workflow paste import waits for the refreshed row without starting a competing navigation. Mobile merge-queue recovery keeps its fixture turn active until the queue-removal transition has been observed.

The failures were reproduced locally. The preview, Office, and workflow tests pass three no-retry repetitions each (9 total); mobile queue recovery passes three no-retry repetitions. E2E backend and web bundle builds pass. Typecheck, focused ESLint, E2E sleep ratchet, whitespace check, and `list-docs.py validate` pass. Exact-head CI and the complete blob audit must confirm zero retries, errors, and unexpected statuses.

## PR #3936 no-retry follow-up after dd62af1

The exact-head Playwright blob audit for `dd62af16265ee35bf1df38670ac86585655d6fb7` found eight retry-only attempts across workflow stepper, Office onboarding, transient retry notices, task workflow layout, a virtualized file tree, Docker launch, and mobile symlink flows. The tests now wait for authoritative WebSocket or fixture readiness, use task-scoped symlink setup, tolerate the supported compact workflow layout, and let the Jira task page own its single idempotent session ensure. The Office retry exposed a lazy system-skill sync path that inserted bundled skills without reapplying role-default skills to an existing CEO. Lazy sync now triggers the existing workspace backfill when it inserts or removes system skills; a focused regression test covers that state. This preserves the existing Office agent onboarding contract, so no Office requirements or design change was needed.

The affected E2E cases pass locally without retries: the five-spec Chromium group passes 45/45 at three repetitions, mobile symlink and transient retry pass 6/6 at three repetitions, Docker slow bootstrap passes 3/3 in host/container mode, and the full Office system-skills file passes 15/15 at five repetitions after the backend fix. Backend skills, agents, and onboarding Go tests pass; Go lint reports zero issues. Web typecheck, focused ESLint and Prettier, E2E sleep ratchet, E2E backend/web build, and `git diff --check` pass. The first build and lint attempts hit `ENOSPC`; after removing the task-owned 711 MB prior-head artifact bundle, both reruns passed. Exact-head CI and its blob audit remain pending.

```text
chromium workflow stepper, workflow change, virtualized file tree, system skills, and transient retry; repeat-each=3, retries=0
45 passed
mobile symlink and transient retry; repeat-each=3, retries=0
6 passed
Docker slow bootstrap in host/container mode; repeat-each=3, retries=0
3 passed
Office system-skills file; repeat-each=5, retries=0
15 passed
go test ./internal/office/skills ./internal/office/agents ./internal/office/onboarding
PASS
golangci-lint run ./... --new-from-rev=852a867fd5c59f5845ca1eaf0291138ebc113660 --timeout=5m
0 issues
pnpm run typecheck; focused ESLint and Prettier; pnpm run e2e:sleep-ratchet
PASS
git diff --check
PASS
```
