---
id: "08-retention-status"
title: "Expose retention, expiry and cleanup outcomes"
status: done
wave: 8
depends_on:
  - "05-recovery-cleanup"
  - "06-plugin-administration"
  - "07-profile-ui"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-005
  - REQ-EXECUTORS-PLUGIN-006
  - REQ-EXECUTORS-PLUGIN-007
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-005.1
  - AC-EXECUTORS-PLUGIN-005.2
  - AC-EXECUTORS-PLUGIN-006.1
  - AC-EXECUTORS-PLUGIN-006.2
  - AC-EXECUTORS-PLUGIN-006.3
  - AC-EXECUTORS-PLUGIN-006.4
  - AC-EXECUTORS-PLUGIN-007.2
  - AC-EXECUTORS-PLUGIN-007.3
  - AC-EXECUTORS-PLUGIN-007.4
  - AC-EXECUTORS-PLUGIN-007.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 08: Expose retention, expiry and cleanup outcomes

## Summary

Project effective retention and expiry into profile selection and session environment status. Settle confirmed expiry through existing loss handling and show available recovery actions on desktop and phone.

## In scope

- Compute conservative capabilities from manifest, profile and instance; unknown retention stays unknown and missing optional capabilities stay false.
- Inspect at expiry with bounded retry and distinguish deadline-passed unknown from confirmed expired. Preserve history and require explicit reset after workspace loss.
- Implement UI-03 and provider lifecycle messages, using existing cleanup authorization and localized errors.

## Out of scope

- Backups, automatic replacement, early termination of quiet agents, live AWS tests.

## Acceptance

- Exact expiry remains visible during idle and after reload; no reconnect or Stop extends it.
- Provider timeout never produces false expiry; confirmed expiry disables workspace resume and never allocates a replacement.
- Desktop and phone show matching unavailable, expired, cleanup-pending and uninstall-blocked outcomes with usable authorized actions.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-03: Environment and plugin lifecycle state

Entry: task environment control; plugin settings for disable/uninstall outcomes.

```text
Desktop: environment disclosure
Provider: Example remote       State: Running
Workspace: ends with compute
Expires: Sep 26, 18:00          [Executor settings]

Phone: environment button opens touch drawer
+-----------------------------+
| Environment           Close |
| Example remote              |
| Running                     |
| Workspace ends with compute |
| Expires Sep 26, 18:00        |
| [Executor settings]         |
+-----------------------------+

Changed states, both presentations
Expired: Workspace is no longer available. [Reset environment]
Unreachable: Could not confirm environment state. [Refresh]
Cleanup pending: Resource removal is unconfirmed. [Retry cleanup]
Plugin disabled: Remote resources may still be running. [Plugin settings]
Uninstall blocked: Clean up retained resources before uninstalling.
```

Reuse existing authorized reset/cleanup actions. Do not create a force-forget control.
Show retry only for an authorized pending cleanup; it repeats that admitted operation.
Status data is shared between desktop popover and touch drawer. The drawer owns scrolling.
Exact expiry, retention consequence, and available actions remain visible without hover.
Covers AC-EXECUTORS-PLUGIN-005.1 through 005.4, 006.1 through 006.4 and 007.2 through 007.5.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/handlers ./internal/plugins/... -run 'TestPluginExecutor' -count=1)
(cd apps/web && pnpm exec vitest run components/task/executor-environment-status.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/plugin-executor-status.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-plugin-executor-status.spec.ts)
```

Required evidence:

- `internal/agent/runtime/lifecycle/executor_plugin_status_test.go: TestPluginExecutorExpiry, TestPluginExecutorUnknownRetention`
- `components/task/executor-environment-status.test.ts: expired, unknown, unavailable projections`
- `e2e/tests/session/plugin-executor-status.spec.ts and mobile-plugin-executor-status.spec.ts: UI-03 and lifecycle messages`

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/ (new executor_plugin_status.go and executor_plugin_status_test.go)`
- `apps/backend/internal/task/ session status DTO and projection`
- `apps/web/components/task/executor-environment-status.ts and executor-environment-disclosure.tsx`
- `apps/web/components/task/executor-environment-status.test.ts`
- `apps/web/e2e/tests/session/plugin-executor-status.spec.ts (new)`
- `apps/web/e2e/tests/session/mobile-plugin-executor-status.spec.ts (new)`
- `apps/web/src/locales/`

## Dependencies

[Task 05](task-05-recovery-cleanup.md), [Task 06](task-06-plugin-administration.md), [Task 07](task-07-profile-ui.md).

## Risks

Do not equate a provider deadline or a failed status read with confirmed destruction. Keep remote environment state separate from task state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Implemented the conservative effective-retention projection and persisted provider expiry, bounded expiry inspection, and confirmed-expiry settlement. The safe status projection is available from the live task-environment endpoint. Desktop disclosure and phone drawer show retention, expiry, unavailable and cleanup-pending states, with existing authorized refresh/reset actions. Added plugin-remote task controls to both session surfaces.

Validation passed:

- Focused Go lifecycle, task service, handler and backend-app tests, including bounded inspection and expiry cases.
- Focused frontend status and desktop/phone top-bar tests; frontend typecheck and lint.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
- Managed desktop E2E `plugin-executor-status.spec.ts` and phone E2E `mobile-plugin-executor-status.spec.ts` under `mobile-chrome`.

The Task 09 full race and documentation audits provide the remaining combined verification for shared plugin packages.
