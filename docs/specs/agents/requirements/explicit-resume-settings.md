---
status: draft
system: agents
created: 2026-09-29
owners:
  - kandev
---

# Explicit Resume Without Mode or Model Overrides

## Ownership and scope

Agents owns this capability because it owns provider configuration enforcement
and conversation recovery. This amendment covers Auggie task sessions. Other
providers and Office launch policy retain their existing behavior.

The user requests strict initial startup and ordinary/automatic resume, followed
by an explicit recovery path using the existing Resume control after failure.
The recovery uses the provider's restored settings or defaults, which need not
be the factory defaults and need not equal the configured profile values.

## Requirements

### REQ-AGENTS-EXPLICIT-RESUME-SETTINGS-001: Explicit recovery with provider settings

- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.1:** When an Auggie task starts or resumes ordinarily, Kandev shall enforce the effective configured model and mode before admitting a prompt. An unavailable or rejected model, refused or mismatched mode, or unconfirmed explicit mode shall fail the attempt with a visible reason. Profile model fallback settings shall not authorize continuation on these paths. An unset selection shall retain provider settings.
- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.2:** After a failed startup/resume, clicking Resume shall retry the same Auggie conversation without applying either mode or model overrides. The recovery surface shall explain this behavior before the click. Automatic resume, background retries, ordinary pause/resume, and Start fresh shall not acquire this exception.
- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.3:** Recovery shall preserve the task session, stored provider conversation identity, profile settings, saved session overrides, authentication, workspace, and unrelated permissions. Missing or unusable conversation identity shall remain an error; recovery shall not silently create a fresh conversation.
- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.4:** The override omission shall apply only to the explicit recovery attempt. A later ordinary launch/resume shall enforce the saved settings again. Mode/model changes explicitly made by the user after recovery shall use normal validation and confirmation and remain available wherever the provider advertises them.
- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.5:** On successful recovery, Kandev shall show a persistent session notice that mode/model overrides were skipped and display provider-reported effective values. Unknown values shall remain unknown. Failure shall retain its cause without claiming successful recovery. Reload/replay shall preserve the notice without duplicating it for the same attempt.
- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.6:** Authentication, transport, workspace, cancellation, and conversation-loading failures shall remain errors. A denied or stale recovery request shall not start an agent, erase saved selections, or overwrite a newer session state.
- **AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.7:** Desktop and phone recovery surfaces shall offer the same action and explanation, disable equivalent actions while pending, and retain inline failure feedback. Phone/coarse-pointer controls shall have at least 44px hit targets, keyboard operation shall remain available, and long copy shall not cause horizontal overflow.

## Contract amendments

For the scoped Auggie task start/resume paths, criterion .1 supersedes the
compatible-launch continuation in
[model fallback criteria .3-.5 and .8](no-silent-model-fallback.md).
Criterion .2 is an explicit, attempt-scoped exception to the mode reapplication
obligation in [permission integrity .007.4 and .007.8](permission-control-integrity.md).
It does not redefine confirmation or claim that a requested setting was applied.
[Resume identity preservation](agent-resume-runtime-recovery.md) remains in force.

## Exclusions

No provider switch, shared-settings mutation, automatic fallback, new profile
switch, factory-default reset, relaxed ACP mode confirmation, or guarantee that
an invalid provider conversation can be recovered. Do not expand this change
to other providers or Office without a separate behavior decision.

## Design and delivery

- [System design](../system-design/explicit-resume-settings.md)
- [Implementation package](../../../plans/agent-resume-mode-fallback/plan.md)
