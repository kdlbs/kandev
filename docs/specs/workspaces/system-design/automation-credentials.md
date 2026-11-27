---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-001
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-002
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-003
---

# Automation credentials system design

## Purpose and boundaries

Workspaces own the rules that permit an automation to use an account or workspace credential.
The automation service owns definition changes, trigger admission, dispatch, and run status.
The runtime owns environment merge and provider authentication.
This design adds credential checks to those entry points and retains their lifecycle boundaries.

The [authorization decision](../../../decisions/2026-11-26-automation-credential-authority.md) records the proposed scope.
The [repository secrets design](repository-secrets.md) owns the existing binding and merge rules.
The [automation retry design](../../office/system-design/automation-retries.md) owns run identity and retry scheduling.
[Workspace collaboration](../../tasks/system-design/workspace-collaboration.md) owns general task access.
The credential rules below add a check for protected automation runs at the existing task authorization boundaries.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-WORKSPACES-AUTOMATION-CREDENTIALS-001` | [Creator and validation](#creator-and-validation), [Run and recovery](#run-and-recovery), [Editor and run detail](#editor-and-run-detail) |
| `REQ-WORKSPACES-AUTOMATION-CREDENTIALS-002` | [Credential choices](#credential-choices), [Run access](#run-access), [Storage and portability](#storage-and-portability) |
| `REQ-WORKSPACES-AUTOMATION-CREDENTIALS-003` | [Workspace grant authority](#workspace-grant-authority), [Run and recovery](#run-and-recovery), [Editor and run detail](#editor-and-run-detail) |

## Creator and validation

`internal/automation.Service` already saves `CreatedByUserID`. The backend app uses
`task.Service.PrepareAutonomousExecutor` to restore this identity and check executor access.
Use this seam for candidate and runtime checks. A new candidate uses the signed-in caller as its creator.
Treat the stored creator as immutable. An editor never becomes the creator by saving an existing definition.
Check the interactive caller's permission first. To replace a creator, create a fresh automation and configure its credentials.

With authentication on, an active creator with current launch permission supplies the credential identity.
Use the stored workspace and tenant for the existing `session.prompt` check.
A scheduler identity supplies no independent secret authority.
For a legacy automation without an active creator, stop a secret-bearing launch with an account error.
Preserve access to disable, stop, and delete operations under their existing control permissions.
With authentication off, the existing single-user path handles saved references.

A backend-app adapter gathers references from the effective agent profile, executor profile, and repositories.
Use the launch readers that feed `internal/agent/runtime/lifecycle` and the same environment merge rules.
Use `runtime/environment` to identify the required references after conflicts and precedence.
The reader resolves existing Workspace repository references through the scoped secret store.
Profile references use Global scope. A known provider API-key field uses its existing scoped check.
Resolve explicit remote authentication references when the effective executor exposes them to the existing admission checker.
Reuse the applicable [remote admission rules](../../agents/system-design/remote-auth-admission.md).

Use the existing save and enable operations for the editor check.
Check value readability with the scoped resolver before it enables the automation.
Discard any cleartext used for this check. Provider login and provisioning remain launch operations.
When a dynamic profile, inherited selection, or event-selected repository leaves a source unknown, defer that part of the check.
Check the exact supported source at actual launch.

Check the complete candidate in `CreateAutomation`, `UpdateAutomation`, and `EnableAutomation`.
Enabled creates and changes to executable settings require validation of known sources.
For an enabled automation, use the same check before enabling or replacing a trigger.
A rejected change preserves the saved definition and trigger state.
A disabled save can retain an incomplete selection. New credential choices always require ownership authorization.
Metadata-only changes and disabling remain available.
At launch, validate legacy enabled rows with the same rules. Migration does not edit their schedules.

Return a typed `automation_credentials_unavailable` error with a field and a source that the caller can view.
**Run now** uses the same launch check and run detail as scheduled work.
Use a generic message for a missing or inaccessible reference. This covers both cases without disclosing another account's state.
Treat storage errors as temporary and retain existing error handling for failed reads.

## Credential choices

Keep a credential choice on one automation.
The creator can select one of their Global secrets for an existing secret-backed environment field on the selected shared profile.
Accept a personal binding on the authorized native editor mutation path.
Reuse the current user's Global secret catalog. Shared profile writes retain the existing organization permission.
The creator can remove a personal binding to select the profile default explicitly.

Store at most one credential choice at each source field. Use the kind, source ID, and environment key to identify that field.
Record the source reference that existed when the creator or owner selected it.
When the source or reference changes, require the creator to repair the credential choice before launch.
A replacement value on the same secret record follows the normal rotation rules.
Apply a personal binding to the matching profile field before environment validation.
A Workspace grant supplies authority for the existing repository binding. The reference and the environment merge both stay the same.

### Supported runs

Credential choices require the automation history destination and a new task for each run.
They require fixed profiles and manual or scheduled runs. A Workspace grant also requires a fixed repository.
Reject an incompatible definition or trigger change before saving it, even for a disabled draft.
The same rule applies when a saved definition becomes incompatible. See [Run and recovery](#run-and-recovery).

Existing task-based webhook, provider-event, and continued runs receive validation for their existing references.
Managed conversation delivery uses a separate path outside this validation.
Dynamic selection and provider API-key fields can use their existing references within validation.
The first release offers personal bindings at profile fields and Workspace grants at existing repository bindings.

## Workspace grant authority

The current workspace owner creates a Workspace grant on a saved automation, including a disabled draft.
Require the same account to hold `secret.manage` and own the Workspace secret in the selected repository binding.
Kandev rejects a grant request from either owner when the two identities differ.
The workspace owner can replace that binding with a Workspace secret they own and make a new grant.
Check that the creator still has launch permission and the binding still belongs to a selected repository.
The Workspace grant records the workspace, automation, creator, repository binding, secret, and grantor.
Keep the grantor's active account and workspace ownership as launch conditions.
Invalidate the Workspace grant on binding replacement or workspace ownership transfer.
Use the owning record's revision or mark the grant stale on the write path. Returning to an earlier selection cannot revive it.

Add scoped grant and revoke operations to the automation service and WebSocket transport.
The payload identifies a selected repository and binding key. The service reads the existing secret reference, creator, and scope from persisted state.
Use the expected automation settings and credential choice revision for writes. Report stale changes as a conflict.
The current workspace owner can revoke a Workspace grant. A source change or ownership transfer requires a new Workspace grant.
An unavailable or revoked Workspace grant retains its unavailable state at that repository binding.

Use a separate scoped runtime resolution method in `internal/secrets`.
It accepts the exact authorized launch context from the automation adapter.
It reads only the Workspace secret approved for the matching stored repository binding and run.
It has no settings reveal route. Ordinary secret list, lookup, reveal, copy, and move use their existing controls.
The service exposes safe grant state to the creator and workspace owner. Secret identifiers remain private to the authorized selector.

## Run and recovery

`internal/orchestrator/event_handlers_automation.go` resolves the creator before starting an automation task.
Resolve effective sources for manual and scheduled task launches at the same boundary.
Check the [supported run settings](#supported-runs) before launch uses a credential choice, including after recovery.
Reject an incompatible definition with a credential choice before dispatch, including managed conversation delivery.
For webhook, provider-event, and continued task runs, validate their existing references.
Managed conversation delivery stays outside the existing-reference check.
Retry, deferred start, and cold recreation check the actual run mode and current credential authority.
The check precedes environment materialization, repository setup, and agent process start.
A failed check creates no new agent session or environment.
When a scheduled occurrence has an unavailable credential or inactive creator, record its failure once under the existing occurrence identity.
Leave the schedule enabled and use its existing run history and notifications.
End the attempt without starting a retry for that credential error. Check credentials again at the next scheduled occurrence.
Preserve the scheduler's existing behavior for a temporary persistence error.

Restore the selected automation and real run identity from stored state during recovery.
A retry snapshot and a queued start carry intent. Re-read the current grant and creator before use.
Use a current binding revision and the existing launch admission fence to reject stale attempts.
Serialize the last grant authorization and value read against revocation for that Workspace grant.
A revocation that commits first blocks admission. A process that already received the value keeps its environment.
Use the run access check for every new prompt, resume, or runtime attachment. Recheck current authority after revocation.
Allow the owner and creator to stop that run without requiring access to its secret.
Apply normal cleanup and run retention after stop.

A run with a credential choice gets a fresh environment. Store its protected creator ID on the existing automation run record.
Set the field before any reference goes to the runtime. An unverified run association blocks a protected launch.
Use the same field and run association for live, resumed, and retained run surfaces.
Credential changes apply to the next admission. A warm session from a prior run cannot supply the new authority.
Use the existing activation gate to stop relaunch after revocation or creator removal.

## Run access

Apply the protected creator field to runs with credential choices and to their bound tasks, sessions, and environments.
General task access remains under workspace collaboration.
Apply the creator check to manual activation of an automation with a credential choice and to its executable settings.
Reject a non-creator's request before it changes the executable definition or a personal binding.
The workspace owner manages Workspace grants under the separate grant rules below, and can disable the automation or stop its runs.
The entitled creator controls the definition, prompts, session, filesystem, terminal, preview, and agent or plugin tools for its run.
Use the stored task and run association to check access on HTTP, WebSocket, and MCP paths.
Enforce it where task, session, and environment access is authorized. This covers entry by a direct run or task link.
Actor and grant data in a request provide no runtime authority.

Other members with workspace read access receive safe run status and the error category.
Restrict the transcript, summary from agent output, task session, and environment to the creator.
Apply the same projection to broadcasts, plugin history, and retained history.
This design operates within the executor's configured OS and infrastructure trust boundary.
The agent and its child processes can read an injected environment value.
The owner sees that consequence before granting use.

## Storage and portability

The automation store owns one credential choice record per automation and source field.
Use a relation under `internal/automation` with a cascading automation reference.
The record stores scope kind, reference IDs, creator, grantor when present, source identity, revision, and state.
Store no secret value. The existing automation run record owns the protected creator field.
Register the schema with the required store and conformance checks.
Use SQLite and PostgreSQL migration and transaction patterns already used by the automation store.
Fresh and existing databases start with an empty credential choice relation.

Update a definition and its submitted personal bindings in one transaction.
Serialize credential choice replacement and grant revocation per automation, including across PostgreSQL connections.
Carry explicit unavailable state through source change and secret deletion. Avoid a silent fallback to the profile default.
Extend the existing secret reference check to cover active credential choices. Show only references the caller can view.
Delete credential choices with the automation. Workspace deletion uses its existing cleanup path.
Copy and move create a new secret record without copying grants.

Omit credential choices and their references from generic automation responses, exports, and task metadata.
Use the dedicated credential projection for authorized editor reads.
Export a definition with credential choices as disabled and use the existing export warning to describe required setup.
The receiving workspace reselects credentials before enabling the copy.
No credential values, account secret names outside an authorized selector, or internal error details enter logs or notifications.

## Editor and run detail

Keep credentials within the relevant profile or repository fields in the existing **Then** section.
Definitions without credential references use the standard form. Use the existing save bar and **Enabled** toggle.
When Save fails, bring the first affected field into view, announce its error, and keep the draft.
Give one next action at that field. The creator can turn **Enabled** off to save the draft.
After a failed launch, show a short recovery message in the existing run details.

Under an eligible profile, show **Choose my secret** beside the field key to the creator.
Use the shared [Combobox](../../../../apps/web/components/combobox.tsx) with the creator's Global secret catalog.
One selection updates the draft and closes the picker. The trigger shows the selected name.
Offer **Use profile default** in the same picker when a personal binding exists.
For an empty catalog, point to **Settings > Secrets**. Use the existing **Save changes** action to persist the selection or removal.

At a saved repository binding, show the creator a short status when owner approval is needed, granted, or revoked.
Show **Grant Workspace secret** when the current workspace owner also owns the bound secret.
One confirmation names the saved repository field and creator and explains that the agent can read the secret during a run.
Confirm the existing binding directly. Show its secret name only to the authorized secret owner.
An active Workspace grant shows **Revoke grant** to the current workspace owner. Update the row when the revoke succeeds.
Use the existing Runs view for safe status and the owner's **Stop** action.
All new copy uses i18next and all supported locales.

Use an anchored picker and the existing dialog on desktop. Use compact bottom drawers for the same short actions on phones and coarse pointers.
Use the [mobile menu sheet](../../../../apps/web/components/kanban/mobile-menu-sheet.tsx) for drawer geometry and focus return.
Use one column and a single page scroll owner on phones, with one scroll body inside an open drawer.
Keep the existing save controls reachable above the safe area. Use touch targets of at least 44 px and the surrounding control density on fine-pointer desktops.
Use the same draft and actions at each viewport. Closing an overlay returns focus to its field and keeps the draft.

## Observability

Use existing automation run history for blocked launches and their safe source details.
Report validation and admission reasons with bounded codes. Keep secret values and identifiers out of error details.
Use the existing run outcome reporting and [observability guidance](../../../agent-observability.md).
