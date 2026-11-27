---
id: "03-personal-bindings"
title: "Add personal credential bindings"
status: complete
wave: 3
depends_on:
  - "02-protect-run-access"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-002
acceptance_criteria:
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.1
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.2
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.3
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.5
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.7
system_design:
  - ../../specs/workspaces/system-design/automation-credentials.md
---

# Task 03 Add personal credential bindings

## Summary

Let a creator choose a Global secret for a shared profile field on one automation.
Persist the personal binding on the automation.
Set the protected creator field from task 02 before the new credential path launches a run.

## In scope

- Store a personal binding and its exact profile source with the definition. Add required store and schema replay coverage.
- Add the creator's selector under the affected profile in **Then**. Use the selected reference in the existing environment merge.
- Use the protected run access boundary from task 02. Reject unsupported settings on save and launch.
- Extend secret reference checks and portable export warnings. Document the selection and access rules.

## Out of scope

- Workspace grant writes belong to task 04.
- General runtime authorization belongs to task 02.
- Credential choices for continued sessions, dynamic profile routing, or stored provider API-key fields.

## Acceptance

- A Collaborator selects their Global secret beside the profile field, saves, and runs the automation. The shared profile retains its stored reference. Applies to `002.1` and `002.2`.
- The personal binding survives restart. Source removal gives a recoverable error, and unsupported settings block save or launch. Applies to `002.3` and `002.7`.
- Export omits the personal binding. The receiver opens a disabled definition for review. Applies to `002.5`.

## ASCII UI preview

### UI-02 Personal binding

Entry is **Choose my secret** under an applicable profile in **Then**. See the [full preview](plan.md#ui-02-personal-binding).
Covers `AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.1` through `.3`.

```text
Desktop
Agent profile [ Shared review profile v ]
SERVICE_KEY   [ Choose my secret v ]
              | [ Search ]         |
              | Review key         |

Phone
Agent profile [ Shared review profile v ]
SERVICE_KEY
[ Choose my secret v ]
................ inset drawer .....................
Choose my secret
[ Search ]
Review key
```

One selection closes the picker and shows its name at the field. **Use profile default** appears in the same picker when a personal binding exists.
Point to **Settings > Secrets** if the catalog is empty. Apply changes through the existing save bar.
Check phone hit targets, keyboard selection, and focus return. The form keeps the draft when the picker closes.

## Verification

In a fresh worktree, run `cd apps && pnpm install --frozen-lockfile` before the first frontend command.

Use `internal/automation` `TestAutomationPersonalCredentialBinding` for authority, atomic save, source changes, supported settings, and reload.
Use `internal/automation` `TestAutomationCredentialExport` for a disabled export without references.
Extend the desktop and phone flows from task 01. Use temporary stores and a disposable PostgreSQL database for the conformance run.
The browser tests cover activation of the task 02 access checks through a real personal binding.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/automation -run '^TestAutomation(PersonalCredentialBinding|CredentialExport)$' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}" && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
pnpm --dir apps/web exec vitest run components/automations/automation-credentials.test.tsx
pnpm --dir apps/web e2e:run --host --shards 1 --project auth tests/auth/automation-credentials.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/auth/mobile-automation-credentials.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/automation/{models,store,service,handlers,export_service}.go` and their focused tests
- `apps/backend/internal/backendapp/secret_references.go`
- `apps/backend/internal/persistence/{requiredstores,storeconformance}/` and upgrade fixtures
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`
- `apps/web/components/automations/`, automation API types, and the `automations` locale files
- `apps/web/e2e/tests/auth/{automation-credentials,mobile-automation-credentials}.spec.ts`
- `docs/public/{agents-and-profiles,automation-and-mcp}.md`

## Dependencies

Task 01 supplies the common creator and source check. Task 02 protects the resulting run.

## Risks

- A personal binding requires the protected creator field from task 02 before launch.
- Export must retain the existing non-secret settings and require review of the credential choice on the destination.
- Scope changes must give a blocked state even when a profile default can resolve.

## Parallelism

`sequential`

## Inputs

- [Credential choices](../../specs/workspaces/system-design/automation-credentials.md#credential-choices)
- [Run access](../../specs/workspaces/system-design/automation-credentials.md#run-access)
- `apps/web/components/combobox.tsx` and the authenticated executor test fixture

## Results

| Criterion | Result |
| --- | --- |
| 002.1 | Authenticated desktop and phone E2E select the Collaborator's Global secrets for agent and executor fields; the picker empty state, selected labels, and focus return are checked. |
| 002.2 | `TestAutomationPersonalCredentialBinding` checks persisted automation-owned choices and collaborator redaction. E2E reloads, enables, and runs the automation with the selected choices. Shared-profile isolation is represented by separate profiles in the fixture; no second automation comparison was added. |
| 002.3 | `TestAutomationCredentialRepositorySelectionUsesCurrentDefinition` verifies removed repository bindings are unavailable to old runs and future validation follows current selections. A creator can save a disabled draft, obtain a new approval, and enable it. |
| 002.5 | `TestAutomationCredentialExport` verifies credential-backed exports are disabled, omit credential choices, and carry a re-selection warning. The test inspects YAML and ZIP output for secret IDs and names. |
| 002.7 | Candidate settings tests cover incompatible continuation changes and enabled removal that would fall back to an unavailable profile secret. `TestAutomationCanTriggerNowRequiresCurrentCredentialAuthority` protects manual launch eligibility. Frontend tests cover unsupported trigger and continuation controls. |

Retry and recovery retain current authorized choices alongside the executable snapshot: `TestAutomationCredentialRetryStartUsesCurrentChoicesWithSnapshot` and `TestAutomationCredentialRetryRecoveryUsesCurrentChoices` passed. `TestAutomationCredentialEnvironmentRemoteAdmissionUsesPersonalBinding` verifies remote admission checks the effective personal selection.
