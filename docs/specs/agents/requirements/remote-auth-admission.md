---
status: draft
system: agents
created: 2026-11-08
owners:
  - kandev
---

# Remote agent authentication admission

## Overview

Remote compatibility checks must recognize authentication supplied through an authorized agent profile.
Agents owns this contract because it owns provider authentication methods and agent profile settings.
Executors retains compute admission. Tasks retains workflow routing and transition guards.

This contract fills a missing authentication-admission rule identified by [#5180](https://github.com/kdlbs/kandev/issues/5180).
It does not replace the existing secret-scope or workflow requirements.

## Terms

- **Configured route:** A declared provider authentication method supplied by the selected, authorized runtime settings.
- **Usable binding:** A binding that resolves under the current actor and permitted secret scope, without a missing or empty value.
- **Admission:** Configuration validation before a guarded workflow change. It does not authenticate against the provider.

## Requirements

### REQ-AGENTS-REMOTE-AUTH-ADMISSION-001: Authorized authentication evidence

**Intent:** Admit supported authentication settings without borrowing credentials or accepting unusable secret references.

#### Acceptance criteria

- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.1:** An authorized Claude API-key profile shall pass remote authentication compatibility on SSH without separate executor authentication declarations.
  Its `ANTHROPIC_API_KEY` secret binding must be usable by the current actor.
- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.2:** Admission shall require at least one supported configured authentication route.
  Every required environment binding and every configured authentication binding for the selected agent shall pass validation.
  A valid key shall not hide an invalid required binding, conflicting binding, or malformed authentication configuration.
  Missing, inaccessible, wrong-scope, deleted, and locally revoked bindings shall fail closed.
- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.3:** Task, profile, and executor authorization shall precede credential resolution.
  A preview shall not grant later authority. Apply and launch shall repeat their existing authorization and binding checks.
  A task override shall not authorize private compute or another user's secret.
- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.4:** API-key and OAuth routes shall retain distinct identities and provisioning.
  An API key shall reach the child as `ANTHROPIC_API_KEY` and shall not create OAuth credential files.
  Existing authorized OAuth and selected file routes shall retain their provisioning behavior.
- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.5:** Failed admission shall retain the original task assignment and profile mappings.
  A preview shall create no session, transition, credential file, or provider request.
  Responses, events, and diagnostics shall disclose neither credential values nor secret identifiers.
  Cancellation shall stop validation without a successful compatibility result.
- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.6:** Executor settings shall expose separate API-key and OAuth secret choices on desktop and phone.
  Save and reload shall preserve each method's binding without converting or clearing the other method.
  Labels shall be localized. Both choices shall support keyboard and touch input without horizontal document overflow.
- **AC-AGENTS-REMOTE-AUTH-ADMISSION-001.7:** Admission shall use explicitly configured settings rather than host login state or ambient host credentials.
  A secret's provider-side validity shall remain an execution-time concern. Admission shall not claim that a provider accepted the credential.

## Existing contracts

- [Repository and workspace secrets](../../workspaces/requirements/repository-secrets.md), especially AC-001.2, .4, and .10, owns scope and missing-binding behavior.
- [Private compute admission](../../executors/requirements/per-user-ownership.md), especially AC-OWNERSHIP-004.1, owns executor authority.
- [Change workflow](../../tasks/requirements/change-workflow.md), AC-CHANGE-WORKFLOW-002.2 through .5, owns atomic mappings and session lifecycle preservation.
- [Workflow agent overrides](../../tasks/requirements/workflow-agent-overrides.md), AC-001.5, owns replacement eligibility.

## Out of scope

Provider login or network validation during preview, host login copying, shared secret grants, new secret scopes, and repository-only authentication discovery.
There is no model alias resolution, fallback-policy change, session-policy change, or new remote probing API.

## Delivery

- [System design](../system-design/remote-auth-admission.md)
- [Planning package](../../../plans/claude-profile-admission/plan.md)
