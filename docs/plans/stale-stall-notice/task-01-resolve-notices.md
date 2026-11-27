---
id: "01-resolve-notices"
title: "Resolve resumed-turn notices"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.6
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
---

# Task 01: Resolve resumed-turn notices

## Summary

Derive running advisory notice resolution from later agent activity in the
same session and turn. Preserve terminal diagnostics and existing controls.

## In scope

- A pure timestamp/activity predicate and a boolean store selector.
- Live-update regression coverage and desktop/mobile hydration evidence.
- Whole-turn backend read projection and browser evidence for unloaded tool updates.
- Synthetic before/after screenshot assets outside the PR branch.

## Out of scope

Changing watchdog thresholds, cancelling quiet work, or new backend events.

## Acceptance

1. An existing compaction tool row updated after the notice hides it while
   the session and active turn remain running, including after reload.
2. Agent text, reasoning, tools, plans, and permission rows resolve it;
   user/system rows and other sessions/turns do not.
3. Terminal error diagnostics remain available.
4. Updates outside the newest history page resolve the notice live and after
   reload; stale hydration cannot undo known resolution.

## ASCII UI preview

UI-01 (desktop and phone), from [the plan](plan.md#ascii-ui-preview):

```text
Quiet:    Still waiting on Compact conversation.  [Cancel turn]
Resumed:  <completed tool or latest agent content>
```

AC-AGENTS-AGENT-STALL-RECOVERY-001.6 owns visibility. Existing phone control
geometry remains in place.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx lib/state/slices/session/running-notice-activity.test.ts lib/state/slices/session/session-slice.update-messages.test.ts lib/state/slices/session/message-signature.test.ts lib/state/slices/session/session-slice.merge-messages.test.ts lib/state/slices/session/session-slice.upsert.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/action-message.tsx components/task/chat/messages/action-message-state.ts lib/state/slices/session/running-notice-activity.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/message-signature.ts)
(cd apps/backend && go test -race ./internal/task/service ./internal/task/repository/sqlite -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --project chromium e2e/tests/session/stall-notice-recovery.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-stall-notice-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Preserve the desktop capture outside `.pr-assets` before the second runner,
then merge and validate manifests before publication.

## Files likely touched

- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message-state.ts`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/lib/state/slices/session/running-notice-activity.ts`
- `apps/web/lib/state/slices/session/running-notice-activity.test.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/message-signature.ts`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_running_notice.go`
- `apps/backend/internal/task/repository/sqlite/message_running_notice.go`
- `apps/web/e2e/tests/session/stall-notice-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-stall-notice-recovery.spec.ts`

## Dependencies and parallelism

None. Sequential, primary session.

## Inputs

[Requirement](../../specs/agents/requirements/agent-stall-recovery.md) and
[design](../../specs/agents/system-design/agent-stall-recovery.md).

## Risks

Keep timestamp precision and distinguish agent activity from status traffic.

## Results

- RED: the compaction-row live-update assertion failed in the existing
  action-message harness and in Chromium E2E before the production change.
- GREEN: targeted Vitest, 129 tests passed across six files after review remediation.
- TypeScript typecheck and targeted ESLint passed with no errors or warnings.
- Desktop Chromium E2E passed for both loaded and paginated-out compaction
  tools, including live resolution and reload. Phone mobile-chrome passed with
  a 44px Cancel turn target, inline geometry, and reload.
- Backend service/repository race tests passed. The new query passed on real
  SQLite and PostgreSQL; SQL guard and store conformance passed.
- Backend changed-code lint passed with zero issues.
- Specification catalog validation, specification lint, and diff whitespace
  checks passed.
- Fresh synthetic quiet/resumed screenshots captured for both viewports;
  screenshot binaries are published only on a separate media ref.
- Review RED: unloaded-tool live updates failed in both reducer actions; a
  backend paginated read also left the notice unresolved before the projection.
- PR CI, automated review, and merge are tracked in the platform task plan.

## Final history-window proof

The corrected rendered-text assertion exposed automatic history loading when
all padding rows were hidden logs. Seed visible earlier tool activity instead,
verify the compaction row is present in the short case and absent in the long
case, then verify live resolution and reload. The action renderer harness also
covers an unloaded tool update with only the notice in the message store.
Both Chromium cases and the focused frontend checks pass.
