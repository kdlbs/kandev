---
id: "02-recovery-card-causes"
title: "Recovery card workspace and operation causes"
status: done
wave: 2
depends_on:
  - "01-initial-target-unrouted-retry"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.18
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.28
system_design:
  - ../../specs/agents/system-design/recovery-workspace-availability.md
---

# Task 02: Recovery card workspace and operation causes

## Summary

Hide **Restore read-only workspace** on the recovery card, stopped banner, and
action-message surfaces when no checkout exists, and explain why on the card.
Title a failed fresh start or runtime retry by its own operation instead of
**Failed to resume session**.

## Files

- `apps/backend/internal/orchestrator/event_handlers_agent.go`: next to
  `"recovery_actions": true`, set `meta["workspace_restorable"]` from the
  session's task environment. The value is `true` when the status is `""`,
  `ready`, or `stopped` and `WorkspacePath != ""`. Leave the key out on a
  read error.
- A new Go test file beside the existing `event_handlers_agent` tests:
  `TestSessionFailureMetadataWorkspaceRestorable`.
- `apps/web/components/task/chat/session-recovery-model.ts`: skip restore when
  `model.metadata?.workspace_restorable === false`. Show the prerequisite line
  `task:restoreUnavailableNoWorkspace`. Use the operation in
  `recoveryFailureCopy`.
- `apps/web/components/task/chat/session-stopped-banner.tsx` and
  `messages/action-message-recovery.tsx`: pass through workspace availability,
  skip restore when it is `false`, and use the shared operation-based label.
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`: record
  `operation: "fresh_start"` for failed `fresh_start` and `runtime_retry`
  actions. Extend the `ManualSessionRecoveryFailure` operation type.
- Every locale `task.json` under `apps/web/src/locales`: add
  `failedToStartFreshSession` and `restoreUnavailableNoWorkspace`.
  Regenerate `pseudo`. Do not use em dashes.

## Steps

1. Extend `session-recovery-model.test.ts`: `workspace_restorable: false`
   removes the restore choice and shows the localized prerequisite; an absent
   value keeps the current behavior.
2. Extend the recovery-actions hook test: failed `fresh_start` and
   `runtime_retry` actions record operation `fresh_start`.
3. Add the Go metadata, recovery-model, and hook tests. Run them and confirm
   the missing availability and operation cases fail.
4. Implement the metadata and UI behavior.
5. Add action-message and stopped-banner label coverage, plus the phone
   Playwright fixture using persisted recovery metadata. Cover the missing
   workspace restore choice on both surfaces. Run the commands below and
   record lifecycle evidence.

## Commands

```bash
cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestSessionFailureMetadata|TestHandleAgentFailure' -count=1
cd ../.. && bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_agent.go
cd apps && pnpm --filter @kandev/web test -- session-recovery use-session-recovery-actions action-message-recovery session-stopped-banner
pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint
cd web && pnpm run i18n:check && pnpm run i18n:pseudo
pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-workspace-unavailable-recovery.spec.ts
```

## Acceptance

- With no checkout, the card shows no restore button on desktop or phone, and
  shows the localized explanation instead.
- A refused fresh start reads **Failed to start fresh session**, with the
  sanitized cause in the collapsed details.
- Existing recovery-card assertions continue to pass.

## Lifecycle evidence

| Scenario | Owning criterion | Observable invariant | Regression evidence | Command and result |
| --- | --- | --- | --- | --- |
| Restart and recovery | `006.18` | Persisted `workspace_restorable: false` keeps restore hidden on the card, stopped banner, and action message; the card shows its prerequisite after reload | `mobile-workspace-unavailable-recovery.spec.ts` seeds a failed session and reloads the persisted card; stopped-banner and action-message tests assert their restore choices are absent | `pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-workspace-unavailable-recovery.spec.ts` — passed, 1 test; browser tests passed |
| Entry paths | `006.2`, `006.28` | Failed `fresh_start` and `runtime_retry` preserve `fresh_start` as the operation for card and transcript labels | Recovery-action hook, recovery-model, action-message, and stopped-banner tests | `pnpm --filter @kandev/web test -- session-recovery use-session-recovery-actions action-message-recovery session-stopped-banner` — passed, 33 files / 310 tests |
| Ownership changes | `005.3` | A session or environment owned by another task does not claim a restorable workspace | `TestSessionFailureMetadataWorkspaceRestorable` checks both task identity mismatches | `go test -trimpath ./internal/orchestrator -run 'TestSessionFailureMetadata|TestHandleAgentFailure' -count=1` — passed |
| Pause and resume | — | Not applicable: no accepted work or pause/resume behavior changes | Excluded; this work only projects workspace availability and failure operation | — |
| Duplicate and stale delivery | — | Not applicable: no dispatch or replay behavior changes | Excluded; metadata is attached to the failure record | — |
| Concurrent or merged work | — | Not applicable: no launch or merge behavior changes | Excluded; availability is a read-only environment projection | — |

## Results

- `pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint` — passed.
- `pnpm --filter @kandev/web test -- session-recovery use-session-recovery-actions action-message-recovery session-stopped-banner` — passed, 33 files / 310 tests; this includes the stopped-banner and action-message workspace-unavailable cases.
- `pnpm run i18n:check` — passed; all seven supported locales are complete and pseudo is in sync.
- `pnpm run i18n:pseudo` — passed; generated pseudo values for both new keys.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/ceiling_entry.go apps/backend/internal/orchestrator/initial_recovery_prompt.go apps/backend/internal/orchestrator/fresh_start_initial_target_test.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/event_handlers_agent_recovery_workspace_test.go` — passed, 0 issues.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` — passed.

## Out of scope

New recovery actions, a new error-code taxonomy, and restore behavior when a
workspace exists.
