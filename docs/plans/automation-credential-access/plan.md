---
created: 2026-11-26
status: complete
requirements:
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-001
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-002
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-003
system_design:
  - ../../specs/workspaces/system-design/automation-credentials.md
legacy_specs: []
---

# Implementation plan for automation credential access

## Overview

The first change reports unusable credentials before a creator enables an automation.
The next change protects access to runs that use a credential choice.
The third change lets a creator select a personal secret for an existing profile field.
The final change adds an explicit Workspace secret grant.
This order gives users earlier feedback while the new credential paths are in development.

Use the [requirements](../../specs/workspaces/requirements/automation-credentials.md),
the [system design](../../specs/workspaces/system-design/automation-credentials.md),
and the [proposed ADR](../../decisions/2026-11-26-automation-credential-authority.md).
The workspace system owns this package because it owns secret scope and runtime use.
The automation, agent, executor, and repository code provide the entry points.

## Evidence

The isolated v0.103.0 reproduction used a fresh SQLite database, separate Owner and Collaborator accounts, and one shared Local executor.
The test used authenticated requests for **Save** and **Run now** and a mock agent.

| Account | Credential | Save | Run |
| --- | --- | --- | --- |
| Collaborator | No secret reference | Succeeded | Succeeded |
| Owner | Owner's Global secret on a shared agent profile | Succeeded | Succeeded |
| Collaborator | Same shared profile and Global secret | Succeeded | Unavailable secret |
| Owner | Owner's secret copied to Workspace scope and bound to a repository | Succeeded | Succeeded |
| Collaborator | Same repository and Workspace secret | Succeeded | Unavailable secret |

The temporary database confirmed that both secret records had the same owner.
The two failures occurred before an agent session started.
The test establishes the failure path. The reported installation's exact settings and error are unknown.

The existing repository contract inherits references and checks the launch account before it resolves them.
The store test `TestSQLiteStore_ScopedSecretVisibility` covers the account check within one workspace.
This package clarifies that contract and proposes a separate grant for one automation. [Repository secrets requirements](../../specs/workspaces/requirements/repository-secrets.md).

## Scope

### In scope

- Credential validation for known fields during save and enablement. Current access checks before launch for each effective source.
- Disabled drafts and actionable errors on desktop and phone after the existing Save and Run actions.
- Protected access to a run that uses a credential choice and its task, session, and environment.
- Creator-selected Global secrets for existing secret-backed fields on shared agent and executor profiles.
- Workspace grants for one existing repository binding on one saved automation.
- Existing launch, retry, cleanup, and authorization paths for the supported runs.
- Public documentation for credential selection, portability, and revocation.

### Design assumptions for review

- A personal binding belongs to one automation. A shared profile can serve several automations with independent personal bindings.
- A Workspace grant requires the current workspace owner to own the exact secret in the repository binding.
- Credential choices apply to manual and scheduled runs that create a fresh task in automation history.
- The creator is fixed. Another account can start from a new disabled copy that requires credential review.
- The creator can interact with the resulting session. The workspace owner can stop it and manage their grants.
- A legacy enabled automation with an unavailable credential retains its definition. Its next launch reports the applicable error.

### Out of scope

- Repository-wide grants, Workspace grants on profile fields, and credential sharing on ordinary tasks.
- Credential choices for webhook, provider-event, and continued task launches. Those modes receive the existing-reference checks.
- Managed conversation credential validation and provider sign-in changes.
- Secret values in exports, settings changes on behalf of another account, and OS-level isolation changes.
- Broader shared-profile and workflow work tracked in [issue #5167](https://github.com/kdlbs/kandev/issues/5167).

## Technical approach

### 01 Validate and report credentials

Add a credential-validation dependency in `internal/automation` and wire it in `internal/backendapp`.
Read the effective references with the same profile and repository readers as runtime.
Use the scoped secret store and the current environment merge rules.
Use the existing enabled save and enable operations to check value readability. The normal launch path repeats the check.
Return safe source details through the existing WebSocket and run error paths.
Default the editor to its existing **Enabled** behavior. Let the creator turn it off and save a draft.

### 02 Protect credential run access

Add a private creator field to the existing automation run record for runs with a credential choice.
Apply the restriction through existing task, session, and environment authorization.
Filter run output and history at each transport. Keep safe status and owner Stop available.
Verify this boundary with authorized fixtures before the editor offers personal bindings or grants.

### 03 Add personal bindings

Add one automation-owned relation for credential choices.
Keep binding writes atomic with definition writes.
Use the existing combobox beside the affected profile field. Read its options from the creator's Global catalog.
Apply the personal binding and set the protected creator field before launch.
Keep portable exports free of credential choices and mark copies disabled.

### 04 Add Workspace grants

Reuse the credential choice record and the dedicated editor projection.
Add owner grant and revoke operations for a saved automation's existing Workspace repository binding.
Check both ownership roles on the granting account. Resolve the approved reference for the current creator and exact binding.
Check the current grant before queued or replayed admission. Keep an unavailable state after revocation.
Update the required store conformance scenarios for both SQLite and PostgreSQL.

### Credential source coverage

| Source | Validation in 01 | Credential choice | Evidence |
| --- | --- | --- | --- |
| Standard agent or executor profile environment | Check the required stored references | Personal binding in 03 | API and browser owner/collaborator controls |
| Fixed repository binding | Check its Global or same-workspace reference | Workspace grant in 04 for an existing Workspace reference | Runtime, revoke, and browser scenarios |
| Provider API-key field or executor authentication method | Check known references through the existing resolver | Existing selection | Scoped read regression, no provider network call |
| Dynamic profile or event-selected repository | Check known sources and mark the unresolved part for launch | No credential choice | Deferred source test and exact launch check |
| Managed conversation delivery | Excluded from task 01 | No credential choice | Target-mode preservation test |

## ASCII UI preview

The labels and spacing are illustrative. The control order, access rules, focus behavior, and recovery actions are requirements.
Keep the existing **Then** form group and settings save bar. Put the credential action beside its profile or repository. Automations without credential references use the standard form.
On a phone, the editor owns page scroll. A short credential selection or confirmation opens an inset drawer with safe-area padding and one scroll body.
Use the mobile menu sheet for geometry and focus return.

### UI-01 Credential status and recovery

Entry is **Settings > Workspaces > workspace > Automations > New or Edit**.
This view shows the existing form after an enabled save reports an unavailable credential.
This view covers `AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.2`.

```text
Desktop
Then
  Agent profile    [ Shared review profile v ]
  SERVICE_KEY is unavailable. Choose another profile.
  Executor profile [ Local v ]
  Repository       [ billing v ]
  Enabled [ On ]                          [ Save changes ]

Phone
Then
  Agent profile [ Shared review profile v ]
  SERVICE_KEY is unavailable.
  Choose another profile.
  Executor profile [ Local v ]
  Repository [ billing v ]
  Enabled [ On ]
-------------------------------------------
  [ Save changes ]
```

Bring the first affected field into view after a failed Save and keep the draft. Turn **Enabled** off to save it.
Use the existing Save progress while the request runs.
After a run fails, the existing run details show the affected source and **Edit automation** if the viewer can edit it.
The rendered phone check covers one scroll axis, usable touch targets, and recovery without losing the draft.

### UI-02 Personal binding

Entry is **Choose my secret** under an applicable profile in **Then**. The creator sees it beside the field key.
This view covers `AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.1` through `.3`.

```text
Desktop
Then
  Agent profile [ Shared review profile v ]
  SERVICE_KEY   [ Choose my secret v ]
                | [ Search ]         |
                | Review key         |

Phone
Then
  Agent profile [ Shared review profile v ]
  SERVICE_KEY
  [ Choose my secret v ]
..................... inset drawer .....................
  Choose my secret
  [ Search ]
  Review key
```

When the catalog is empty, point the creator to **Settings > Secrets** to add a Global secret.
Selecting a secret closes the picker and shows its name in the trigger. **Use profile default** appears in the picker when a personal binding exists.
Use **Save changes** to persist either choice. Closing the picker returns focus to the field and keeps the draft.

### UI-03 Workspace grant

Entry is **Grant Workspace secret** under the repository in **Then** for a saved binding.
On a new automation that needs approval, the creator saves with **Enabled** off. The owner opens that saved automation to grant use.
Show the action when the current workspace owner also owns the bound Workspace secret.
This view covers `AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.1`, `.3`, and `.5`.

```text
Desktop
Then
  Repository billing
  SERVICE_KEY     [ Grant Workspace secret ]
  After grant     Approved for Alex  [ Revoke grant ]

Phone
Then
  Repository billing / SERVICE_KEY
  [ Grant Workspace secret ]
..................... inset drawer .....................
  Grant Workspace secret
  For Alex on this automation
  Repository billing / SERVICE_KEY
  Billing key
  The agent can read this secret during the run.
  [ Grant use ]
```

Use the same confirmation content in the desktop dialog. The confirmation uses the saved binding and shows its secret name only to the authorized secret owner.
For a creator who cannot grant, show **Owner approval needed**, **Approved**, or **Grant revoked** beside that binding as applicable.
**Revoke grant** applies the change immediately and updates the status. The owner can use **Stop** in Runs if the job is active.

## Tests

Use synthetic secrets in temporary stores. Reuse the normal authentication and automation entry points.

| Criteria | Executed backend tests |
| --- | --- |
| 001.1, 001.4, 001.7 | `TestAutomationCredentialSave`, `TestAutomationCredentialEnvironment`, `TestAutomationCredentialLaunch`, and `TestLifecycleAutomationCredentialBindingReaderAllowsUnprotectedRun`. |
| 001.2 | Authenticated desktop and phone E2E flows for inline errors and disabled drafts. |
| 001.3, 001.5 | `TestAutomationCredentialActorRequiresActiveCreatorAndPromptScope` covers missing, inactive, and demoted creators; `TestAutomationCredentialScheduledLaunchRecordsUnavailableCreatorOnce` covers one failure per scheduled occurrence. |
| 002.1 through 002.3 | `internal/automation` `TestAutomationPersonalCredentialBinding`; authenticated desktop and phone selection/reload/run flows. |
| 002.4, 002.6 | `TestAutomationCredentialRunAccess`, `TestAutomationCredentialRunAccessProjection`, `TestAutomationCredentialRunLaunchKeepsPrivateChoices`, and `TestCredentialAuthorityIsRecheckedAtAutomationUpdateCommit`. Plugin history, workspace event broadcast, retained task cleanup, and owner Stop use dedicated regression tests. |
| 002.5 | `internal/automation` `TestAutomationCredentialExport`. |
| 003.1 through 003.5 | `TestAutomationWorkspaceSecretGrant` covers binding and ownership revisions, stale confirmation, ownership changes away and back, and stale revocation. Run/retry commit checks and live prompt/runtime revocation have dedicated tests. SQLite and disposable PostgreSQL store conformance passed with `-race`. |

## E2E tests

Use `apps/web/e2e/tests/auth/automation-credentials.spec.ts` in `auth` and
`apps/web/e2e/tests/auth/mobile-automation-credentials.spec.ts` in `mobile-chrome`.
Use the mock agent, the authentication fixtures, and a fresh database per worker.
Exercise each step through the real editor or Runs view.

| Flow | Criteria | Desktop and phone evidence |
| --- | --- | --- |
| Owner secret blocks enabled save and the collaborator can save a disabled draft | 001.1, 001.2, 001.4 | Inline error, preserved draft, and successful disabled Save |
| Creator selects a personal secret and runs the definition | 002.1, 002.2, 002.3, 002.4 | Phone drawer, selection, reload, successful run, return to profile default, safe history from the owner account |
| Owner grants a bound Workspace secret and revokes it | 003.1, 003.2, 003.3, 003.5 | Grant confirmation, creator run after grant, updated status after revoke, owner Stop, phone drawer and role controls |

The [runner guide](../../../apps/web/e2e/README.md) describes the managed build, worker isolation, and cleanup.
If the live instance uses the checkout's build products, use an isolated checkout for implementation verification.

## Rendered UI review

Completed in isolated Playwright fixtures with authentication enabled, a fresh test database per run, separate Owner, Creator, and other-member accounts, a temporary local repository, and a mock agent. The fixture starts and stops its own backend and did not use the user's running instance or database.

The desktop and phone flows rendered and exercised the field error, disabled draft Save, personal-secret picker and empty state, owner grant confirmation, granted status, creator enablement and run, owner Stop, revoke status, and the no-secret path through a successful run. The other member received safe run status with no Run now or Stop control. The owner could stop a protected live run without receiving conversation output. The phone flow checked that the affected field and drawer controls fit the viewport and that primary controls meet the fixture's touch-target size. Picker dismissal returned focus to the field and kept the draft. Captures were inspected after animations and notifications settled.

Reviewed captures: `desktop-credential-error.png`, `desktop-personal-secret-picker.png`, `desktop-workspace-grant-confirmation.png`, `desktop-workspace-granted.png`, `desktop-owner-run-stop.png`; and the corresponding `phone-*.png` captures plus `phone-member-safe-run-status.png` and `phone-owner-run-stop.png` in the isolated Playwright result directories. The owner captures show safe status and Stop guidance without a transcript.

The desktop flow exercised the picker with pointer selection and keyboard dismissal. Keyboard selection itself, safe-area measurements, and a separate narrow fine-pointer viewport were not independently checked.

## Work orders

- [x] [Task 01 Validate automation credentials](task-01-validate-credentials.md)
- [x] [Task 02 Protect credential run access](task-02-protect-run-access.md)
- [x] [Task 03 Add personal credential bindings](task-03-personal-bindings.md)
- [x] [Task 04 Grant Workspace secret use](task-04-workspace-grants.md)

Run 01, then 02, then 03, then 04. Each user-facing work order owns the public docs for its change.

## Risks

- Validation can drift from live environment merge. Use the same source readers and merge policy for both stages.
- An enabled schedule can record one failed run per occurrence until an account or credential problem is repaired. The schedule keeps its cadence.
- An admitted process can retain an injected credential after revoke. Check the Stop path and describe the limit in Team Access.
- Task, session, and plugin entry points must honor the protected creator field before the credential selectors ship.
- The draft profile-sharing PR overlaps some runtime seams. Check its status before implementation. [Draft PR #5184](https://github.com/kdlbs/kandev/pull/5184).

## Implementation results

Implementation and the rendered review completed on 2026-11-27. The v0.103.0 reproduction above remains the baseline evidence.

- `make -C apps/backend build` passed, including the backend and its agent binaries. The authenticated E2E runner also built the production web/backend fixture before the final desktop and phone runs.
- Focused Go regressions passed across automation, backend app, lifecycle, orchestrator, task service, and WebSocket packages. The broad `internal/automation` and isolated `internal/orchestrator` package suites passed. SQLite and PostgreSQL store conformance passed with `-race`; PostgreSQL ran in an isolated temporary Docker container that was removed after the test.
- The targeted regressions cover ordinary unprotected runs, missing/inactive/demoted creators, scheduled failure recording, retry start and recovery choices, remote personal bindings, plugin history, WebSocket broadcasts, cleanup failure, live revocation, stale grants and ownership revisions, and commit-time authority rechecks.
- The focused frontend suite passed: 6 files and 95 tests. Web typecheck, Oxlint, and locale checks passed.
- Desktop E2E passed (1 test, 25.4s); phone E2E passed (1 test, 42.2s). Both used fresh isolated databases and distinct Owner, Creator, and other-member accounts. Each run exercised the no-secret successful launch, creator launch, grant and revoke, safe member projection, and owner Stop.
- SQLGuard passed. Public-doc tests (63 passed), public-doc validation (56 pages), specification catalog validation (467 decisions and 2,120 specs), specification lint, `git diff --check`, and the Go formatting check passed.
