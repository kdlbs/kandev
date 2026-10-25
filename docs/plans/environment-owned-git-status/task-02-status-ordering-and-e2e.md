---
id: "02-status-ordering-and-e2e"
title: "Reject older environment status"
status: done
wave: 2
depends_on:
  - "01-canonical-status-source"
plan: "plan.md"
spec: "../../specs/tasks/requirements/additional-session-workspace-reuse.md"
design: "../../specs/tasks/system-design/environment-owned-git-status.md"
---

# Task 02: Reject Older Environment Status

## Goal

Prevent a late, older Git-status event from replacing newer state in the shared
environment cache, and prove the user-visible Changes result across sibling
session hydration.

## Scope

- Add timestamp ordering to `applyGitStatus` for each environment and repository
  key.
- Keep the newest valid timestamp as a watermark, including when Git content is
  unchanged.
- Reject older dated entries. Also reject undated entries when the current
  state has a valid timestamp.
- Keep equal-time meaningful updates compatible.
- Extend focused WebSocket/store tests.
- Add or extend a two-session Chromium E2E test. Create an uncommitted file,
  reload the task, and allow both sessions to hydrate. Switch between the
  sibling tabs, and prove that the file remains in Changes.

## Exclusions

- Do not add layout, touch, navigation, or user-copy changes.
- Do not add a special rule that preserves non-empty file lists.
- Do not remove or conditionally mount existing session subscriptions.
- Do not add a separate mobile implementation or duplicate mobile E2E case.

## Requirements

- `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001`
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5`

## RED regressions

First add a unit test that maps two sessions to one environment. Deliver a
newer dirty status through one session and an older clean status through the
other. Prove that the current store incorrectly becomes clean before the fix.

Then add the user regression in
`apps/web/e2e/tests/session/session-tab-management.spec.ts`. Confirm that the
test reaches both session hydrations and observes the dirty file through the
Changes UI.

Suggested unit test name:

- `does not overwrite newer environment status with an older sibling event`

## Acceptance

- Older or unknown-age events cannot replace a newer dated environment status.
- Duplicate content can advance the timestamp watermark without invalidating
  the cumulative diff cache.
- The Changes panel keeps the uncommitted file visible through reload and a
  sibling-session switch.

## Verification

```bash
cd apps/web && pnpm test -- lib/ws/handlers/git-status.test.ts
make -C apps/backend build
cd apps/web && pnpm run build:e2e
cd apps/web && pnpm e2e:run tests/session/session-tab-management.spec.ts --project=chromium -g "keeps dirty Changes status after sibling hydration"
```

## Files likely touched

- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-state.ts`
- `apps/web/lib/ws/handlers/git-status.test.ts`
- `apps/web/e2e/tests/session/session-tab-management.spec.ts`

## Dependencies

Task 01. Rebase on PR #3167 before delivery if it merges first.

## Mobile parity

Desktop and mobile Changes surfaces read the same status store. This task does
not change presentation or interaction. The unit regression proves the shared
data rule, and the Chromium E2E test proves the user-visible flow. No
mobile-specific behavior needs a second browser case.

## Output contract

Report RED and GREEN evidence, timestamp handling for invalid and equal values,
changed files, unit and E2E results, and any observed flake risk. Update this
work order and `plan.md` in the implementation turn.

## Results

- RED evidence: the shared-environment handler test allowed an older clean
  sibling event to replace the newer dirty status before timestamp ordering
  was implemented.
- GREEN evidence: the focused handler, session-runtime return-value, and
  multi-repository tests passed 25 tests. The exact work-order command
  `pnpm test -- lib/ws/handlers/git-status.test.ts` also passes.
- Valid timestamps are monotonic per environment and repository. Older valid
  timestamps and undated entries after dated state are rejected. Equal-time
  content changes remain accepted, and newer duplicate content advances the
  watermark without invalidating the cumulative diff cache.
- The managed Chromium regression passed: it reloads the shared task, waits
  for both sibling `session.git.event` hydrations, and keeps the dirty file
  visible in Changes after switching sessions. No flake or retry was needed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run build:e2e`, and the focused
  Chromium E2E command passed. Mobile uses the same store and has no
  presentation or interaction change, so no duplicate mobile scenario was
  added.
