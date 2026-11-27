---
id: "01-validate-credentials"
title: "Validate automation credentials"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-001
acceptance_criteria:
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.1
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.2
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.3
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.4
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.5
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.6
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.7
system_design:
  - ../../specs/workspaces/system-design/automation-credentials.md
---

# Task 01 Validate automation credentials

## Summary

Give the creator actionable credential feedback before enablement.
Check the same creator and effective references again at launch.
This slice delivers the fix for the v0.103.0 failure path with the existing secret model.

## In scope

- Collect existing agent, executor, and repository references through the runtime selection readers.
- Check credentials through the existing enabled save, trigger enablement, and launch operations. Use the same launch check for **Run now**.
- Use the recorded creator for saved definitions. Require an active creator in authenticated secret-bearing launches.
- Show an actionable source error in the editor or run details. Check run-specific sources at launch.
- Store one terminal credential error per scheduled occurrence and keep the schedule enabled.
- Document disabled drafts and the present scope rules in the automation guide.

## Out of scope

- Run protection, personal bindings, and Workspace grants belong to tasks 02 through 04.
- Managed conversation delivery and provider sign-in.

## Acceptance

- A Collaborator can save a disabled draft with an unavailable secret. The enabled write returns the applicable source error.
- Manual, scheduled, event, deferred, and retry launches of tasks check the current creator and required references before provisioning.
- A blocked Save brings the affected field and its recovery action into view on desktop and phone. The editor keeps the draft. Enabled writes check value readability.

## ASCII UI preview

### UI-01 Credential status and recovery

Entry is **Then** in the automation editor after an enabled save fails. See the [full preview](plan.md#ui-01-credential-status-and-recovery).
Covers `AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.2`.

```text
Desktop
Agent profile [ Shared review profile v ]
SERVICE_KEY is unavailable. Choose another profile.
Enabled [ On ]                          [ Save changes ]

Phone
Agent profile [ Shared review profile v ]
SERVICE_KEY is unavailable.
Choose another profile.
Enabled [ On ]
-------------------------------------------
[ Save changes ]
```

Reuse the editor's save bar and scroll owner. Keep the draft and announce the first field error.
Check the existing profile selector and **Enabled** toggle on a phone and a narrow fine-pointer viewport. The person can turn **Enabled** off and save.
Keep the standard form for an automation without credential references. Show safe recovery text in the existing run details.

## Verification

In a fresh worktree, run `cd apps && pnpm install --frozen-lockfile` before the first frontend command.

Start with the two-account v0.103.0 scenario in the plan.
Use `TestAutomationCredentialSave` for enabled writes, deferred sources, and disabled drafts.
Use `TestAutomationCredentialEnvironment` for source and precedence equivalence.
Use `TestAutomationCredentialLaunch` for entry paths, stale credentials, and replay.
Exercise the active-creator, temporary storage error, and authentication-off cases.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/automation -run '^TestAutomationCredentialSave$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestAutomationCredentialEnvironment$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestAutomationCredentialLaunch$' -count=1 -v)
pnpm --dir apps/web exec vitest run components/automations/automation-credentials.test.tsx components/runs/automation-detail-page.test.tsx
pnpm --dir apps/web e2e:run --host --shards 1 --project auth tests/auth/automation-credentials.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/auth/mobile-automation-credentials.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/automation/{service,handlers,scheduled_admission}.go` and `service_credentials_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go` and `automation_credential_resolution_test.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go` and `automation_credentials_launch_test.go`
- `apps/web/components/automations/automation-editor-save.ts` and the field error and recovery test
- `apps/web/components/runs/automation-detail-page.tsx`
- `apps/web/lib/api/domains/automation-api.ts`, `apps/web/lib/types/automation.ts`, and the `automations` locale files
- `apps/web/e2e/tests/auth/{automation-credentials,mobile-automation-credentials}.spec.ts`
- `docs/public/{automation-and-mcp,websocket-api}.md`

## Dependencies

None.

## Risks

- The runtime can choose a source after save. Check known sources early and the actual sources before launch.
- The editor can save triggers through more than one operation. Check every enabling entry point without changing schedule identity.
- When an older enabled automation has no active creator, surface the error on run and preserve its definition.

## Parallelism

`sequential`

## Inputs

- [Validation and recovery design](../../specs/workspaces/system-design/automation-credentials.md#creator-and-validation)
- [Lifecycle evidence checklist](../../../.agents/skills/tdd/references/lifecycle-evidence.md)
- `apps/web/e2e/tests/auth/executor-ownership-helpers.ts` and `apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts`

## Results

| Criterion | Result |
| --- | --- |
| 001.1 | `TestAutomationCredentialSave` and the full automation package passed. Desktop and phone flows reject unavailable credentials and save the Collaborator's disabled draft. |
| 001.2 | Both authenticated E2E projects show the inline field error, preserve the draft, and complete disabled Save. The phone flow checks the error target is in view. |
| 001.3 | `TestAutomationCredentialLaunch` and `TestAutomationCredentialScheduledLaunchRecordsUnavailableCreatorOnce` passed; the latter asserts one unavailable-creator failure per scheduled occurrence. |
| 001.4 | `TestAutomationCredentialEnvironment`, `TestTaskEnvironmentSources_PreservesRepositoryBindingRevision`, and launch tests passed. The shared fixture reports safe source text without secret identifiers. |
| 001.5 | `TestAutomationCredentialActorRequiresActiveCreatorAndPromptScope` covers missing, inactive, and demoted creators and requires the active account and current workspace prompt scope. `TestLifecycleAutomationCredentialBindingReaderAllowsUnprotectedRun` preserves ordinary runs, while the invalid protected association test fails closed. |
| 001.6 | Existing authentication-off and managed-conversation behavior remains covered by the affected package suites; credential checks are limited to authenticated credential-bearing operations. `TestPrepareAutomationCredentialActorRestoresAuthOffForBackgroundRun` and `TestCredentialValidatorPreparesAuthOffContextForUnprotectedAutomation` cover background auth-off launches. |
| 001.7 | Credential validation and launch checks passed in their focused tests. Desktop and phone E2E assert actionable source details and no secret ID in the field error. |
