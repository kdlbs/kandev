---
status: draft
system: workspaces
created: 2026-11-26
owners:
  - kandev
---

# Automation credentials

## Overview

Workspace owners and collaborators can save automations that use shared profiles.
On v0.103.0, a collaborator could save a definition whose credential belonged to the owner.
The run failed before the agent started. Copying the credential into Workspace scope gave the same result.

The workspace system owns secret scope and authority to use a credential.
The automation editor and scheduler use that authority to check a definition and start its runs.
See [repository secrets](repository-secrets.md) for the existing environment binding contract.

## Terminology

- **Creator** means the account that Kandev records when a person creates an automation.
  The creator is fixed for the life of the automation. To use a different account, create a new automation.
- **Personal binding** means the creator's selection of their Global secret for an existing profile environment field on one automation.
- **Workspace grant** means permission for one automation to use an existing repository binding to a Workspace secret.
  The same account must be the workspace owner and the secret owner.
- **Credential choice** means either a personal binding or a Workspace grant on an automation.

### REQ-WORKSPACES-AUTOMATION-CREDENTIALS-001: Automation credential validation

#### Acceptance criteria

- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.1:** When a person creates an enabled automation or enables an existing one, Kandev shall check known credential sources as the creator.
  The same check applies to changes to executable settings on an enabled automation.
  When a required secret is unavailable, Kandev shall reject the change.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.2:** When validation blocks an enabled save, Kandev shall show the error and a recovery action next to the affected profile or repository.
  The editor shall keep the draft and allow the person to save it with **Enabled** off.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.3:** When a run needs a secret, Kandev shall check the creator's current access before provisioning the environment.
  An unavailable required secret shall produce an actionable run error before agent start.
  This applies to manual, scheduled, and event-triggered task launches that use the supported settings below.
  When a scheduled occurrence fails this check, it shall record one failed run. The schedule shall stay enabled for its next occurrence.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.4:** When a source depends on the actual run, Kandev shall check the known sources at enablement.
  Launch shall check the remaining sources and report any unavailable reference in run details.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.5:** When authentication is on and a secret-bearing automation has no active creator, Kandev shall identify the account problem and stop launch.
  A permitted user can disable the definition or create a fresh one.
  Each scheduled occurrence follows the same cadence and failure recording as an unavailable secret.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.6:** When authentication is off, existing automation saves and runs shall retain their single-user credential behavior.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-001.7:** When an enabled save, enablement, or launch checks a required stored value, Kandev shall validate readability.
  The result shall contain only source details the caller can view and the action to take.

### REQ-WORKSPACES-AUTOMATION-CREDENTIALS-002: Personal bindings on shared profiles

#### Acceptance criteria

- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.1:** A creator with Collaborator access shall be able to bind their Global secret to a supported secret-backed field on a shared profile.
  The creator shall select the secret next to the affected profile in the automation editor.
  The personal binding shall apply to one automation.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.2:** When that automation runs, it shall use the creator's selected secret.
  The shared profile and other automations shall retain their saved values.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.3:** When a selected secret or field becomes unavailable, Kandev shall require the creator to repair or remove the personal binding before the next launch.
  Only the creator can change or remove the personal binding.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.4:** When a run uses a personal binding or Workspace grant, the creator shall control its agent, session, and environment.
  Other workspace members can see safe run status. The workspace owner can stop the run.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.5:** When a person exports or duplicates an automation with a personal binding or Workspace grant, Kandev shall omit that credential choice.
  A new copy shall require review before it runs.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.6:** When an automation has a credential choice, only its fixed creator can change its executable settings or manually start it.
  Kandev shall reject other accounts' requests without changing the definition or credential choice.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.7:** When a credential choice is present, Kandev shall reject saves and trigger changes outside the supported settings below.
  When a stored definition no longer meets those settings, Kandev shall reject launch.

### REQ-WORKSPACES-AUTOMATION-CREDENTIALS-003: Explicit Workspace grants

#### Acceptance criteria

- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.1:** The current workspace owner can grant use of a repository's bound Workspace secret to a saved automation when they also own that secret.
  Each Workspace grant shall identify the automation's creator and the repository binding.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.2:** When the approved automation runs with an active grant, it shall resolve that secret for the approved field.
  Another automation with the same repository shall require its own authority.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.3:** When the owner revokes a grant, Kandev shall block future uses, including queued launches and retries.
  A run that already received the secret retains it until its process stops.
  Kandev shall block new prompts, resumes, and runtime attachments to that run after revocation.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.4:** When a person copies or moves a secret, the destination shall have no grants.
  When workspace ownership or the selected repository binding changes, Kandev shall require a new Workspace grant.
- **AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.5:** At a repository field that supports a Workspace grant, Kandev shall show the approval state to the creator and owner.
  The same field shall show their permitted actions on desktop and phone.
  A grant shall give runtime use only. Only authorized secret settings can reveal or change the value.

## First release

Credential choices require **Run in automation history only** and **Start a new task for every run**.
The first release supports manual and scheduled runs with fixed profiles and a fixed repository selection or a scratch workspace.
A personal binding applies to an existing secret-backed profile field. A Workspace grant requires a selected repository and its existing Workspace secret binding.
Task launches from webhooks, provider events, and continued sessions receive validation for their existing references.
Kandev shall reject a credential choice on these modes and on managed conversation delivery.
Managed conversation delivery with its ordinary configuration uses its existing admission path.

## Out of scope

- Repository-wide grants that apply to several automations, and Workspace grants on profile fields.
- Credential overrides for normal task starts, dynamic agent profiles, provider API-key fields, and managed conversations.
- Changes to provider sign-in, OS-level isolation, or the secret value that an active process already received.
