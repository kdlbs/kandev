---
id: "02-idle-policy"
title: "Workspace policy and automatic recovery"
status: done
wave: 2
depends_on: ["01-conditional-parking"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.1
  - AC-EXECUTORS-IDLE-PARKING-001.2
  - AC-EXECUTORS-IDLE-PARKING-001.3
  - AC-EXECUTORS-IDLE-PARKING-001.4
  - AC-EXECUTORS-IDLE-PARKING-001.5
  - AC-EXECUTORS-IDLE-PARKING-001.6
  - AC-EXECUTORS-IDLE-PARKING-001.7
  - AC-EXECUTORS-IDLE-PARKING-001.8
  - AC-EXECUTORS-IDLE-PARKING-001.9
  - AC-EXECUTORS-IDLE-PARKING-001.10
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 02: Workspace policy and automatic recovery

## Summary

Deliver the workspace setting, expiry scheduler, and automatic message/focus recovery as one functional feature.
Use the shared operation from Task 01 and preserve all existing explicit stop intents.

## In scope

- Add workspace enabled/timeout fields, migrations, DTOs, partial updates, authorization, and SQLite/PostgreSQL parity.
- Add localized workspace Overview controls with enabled=false policy default and a saved 120-minute timeout.
- Wire live policy updates into settlement/scan scheduling; do not reuse the disconnected-agent environment timeout.
- Add known-work checks, candidate generations, repeated-cycle clocks, and bounded diagnostics.
- Route all actionable messages through shared idempotent resume before delivery.
- Add explicit selected-task focus intent for open/selection and browser focus/visibility return; exclude hidden subscriptions.
- Recover only sessions carrying idle-suspension provenance, including the narrow prevent-auto-start exception.
- Preserve tokens, existing restore-failure UI, and message deduplication.

## Out of scope

Provider-specific quiescence APIs, synthetic focus prompts, new admission ceilings, and waking manually stopped sessions.

## Acceptance

1. Every workspace defaults off/120 minutes; validated changes affect only that workspace without restart.
2. Enabled workspaces suspend idle ACP agents, resume on messages or focus, and suspend again after a fresh interval.
3. Concurrent callers resume once; protected sessions and known work remain intact, and no fresh conversation replaces a failed restore.

## ASCII UI preview

UI-01: Workspace settings > Overview > Resource saving. Default state shown.

```text
Desktop
+------------------------------------------------------+
| Resource saving                                      |
| Suspend idle ACP agents                    [Off]     |
| Idle timeout (minutes)            [120, disabled]     |
| Resume when you open the task or it gets a message.   |
|                                            [Save]    |
+------------------------------------------------------+

Phone
+----------------------------------+
| Resource saving                  |
| Suspend idle ACP agents    [Off] |
| Idle timeout (minutes)           |
| [120, disabled                 ] |
| Resume when you open the task    |
| or it gets a message.            |
| [Save                          ] |
+----------------------------------+
```

When enabled, the timeout field becomes editable. An invalid timeout shows an inline error and prevents saving.
Save failures retain edits and expose the existing retry pattern; pending save disables duplicate submission.
Labels, field order, and behavior are structural; spacing and English copy are illustrative and require localization.
Phone uses the existing page scroll owner, 44px touch targets, and stacked controls. Desktop uses 28px controls.
The entry point is the existing workspace overview; no drawer or new navigation layer is needed.
This preview maps to AC-EXECUTORS-IDLE-PARKING-001.1 and .9.

See [the full plan preview](plan.md#ascii-ui-preview). Implement UI-01 on desktop and phone with the same view model.

## Verification

Write the named regressions in the plan first, including workspace defaults/partial updates and all message sources.
Cover hidden-tab events, duplicate focus, restart provenance, policy disable, unsupported restore, pending input, and manual/workflow stop exclusions.
Run PostgreSQL parity tests with the repository's existing database fixture configuration; skips are not parity evidence.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/repository/... ./internal/task/service ./internal/task/handlers -run 'WorkspaceIdlePolicy|IdleSuspension' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'IdleParking|IdleReaper|IdleReclaim' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'IdleParking' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/workspaces/workspace-idle-policy-card.test.tsx hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm exec eslint hooks/domains/session/use-session-resumption.ts hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`, workspace DTO/update handlers and service
- `apps/backend/internal/task/repository/sqlite/workspace.go` and migrations/tests
- `apps/backend/internal/orchestrator/idle_session_reaper.go`, `idle_session_focus.go`, and regression tests
- `apps/backend/internal/orchestrator/idle_session_reaper.go`, `service.go`, message and focus admission paths
- `apps/web/app/settings/workspace/workspace-edit-client.tsx`
- `apps/web/components/settings/workspaces/workspace-idle-policy-card.tsx` and component tests
- `apps/web/hooks/domains/session/use-session-resumption.ts` and tests
- Workspace API types/store and `apps/web/src/locales/` catalogs

## Dependencies

Task 01.

## Risks

Do not wake every session when the user focuses one task. Do not permit hidden tabs or polling to defeat idle expiry.
The new focus exception requires durable suspension provenance, not a generic stopped-state check.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/idle-runtime-parking.md)
- [Design](../../specs/executors/system-design/idle-runtime-parking.md)
- Existing workspace edit card and mobile workspace settings shell
- `/mobile-parity`, `/e2e`, and repository localization guidance

## Results

Added workspace-scoped enabled and timeout settings with disabled/120-minute defaults, validated partial updates, and migration coverage. The orchestrator scans known settled ACP sessions across provider families, excludes known work and protected ownership, retries recoverable shutdowns, and retains suspension provenance for message/focus recovery.

The workspace settings card is localized, follows the existing save flow, and uses the same controls on desktop and phone. Focus and actionable message recovery use the existing resume ownership path; explicit focus is provenance-bound and does not dispatch a prompt. A focus recovery restarts the idle clock.

Regression coverage includes `TestWorkspaceIdlePolicyDefaultsAndPartialUpdates`, `TestWorkspaceIdlePolicyMigrationDefaultsExistingRows`, `TestPostgresIdleSuspensionPolicyAndProvenance`, `TestIdleParkingSuspendsSettledSessionsAcrossACPProviders`, `TestIdleParkingKeepsDisabledAndKnownWorkSessionsRunning`, `TestFocusTaskSessionResumesIdleSuspensionWithNewLSPLease`, and localized component/hook tests. The full backend suite and build, focused PostgreSQL 16 migration/CAS tests, web typecheck, i18n checks, and 40 focused web tests passed.

PR fixup added a deferred focus/startup overlap regression. Browser focus now joins the startup recovery promise for the same request generation, so an overlapping status response cannot issue a second launch from stale suspended state. This implements the existing design rule that concurrent callers join one resume; no requirements or design contract changed. Test cleanup unmounts hooks after each case so focus listeners cannot leak between tests.

Post-fixup validation passed:

```bash
(cd apps/web && pnpm exec vitest run components/settings/workspaces/workspace-idle-policy-card.test.tsx hooks/domains/session/use-session-resumption.test.ts) # 40 passed
(cd apps/web && pnpm exec eslint hooks/domains/session/use-session-resumption.ts hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm run typecheck)
(cd . && python3 scripts/list-docs.py validate) # 322 decisions and 1222 specifications
(cd . && python3 scripts/lint-spec-files.py --all)
```
