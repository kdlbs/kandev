---
created: 2026-11-20
status: implemented
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
legacy_specs: []
---

# Fix plan for strict Claude profile model conflicts

## Finding

[Issue #5183](https://github.com/kdlbs/kandev/issues/5183) pairs the Start model
`sonnet` with `ANTHROPIC_MODEL=claude-sonnet-5-5`. Require exact model is on.
The SSH session advertises the full ID as current and selectable.
The reporter confirmed the same automation started after selecting the full
ID. [Verified workaround](https://github.com/kdlbs/kandev/issues/5183#issuecomment-6720279723)

The settings controller validates that a strict model is present and saves
both inputs. At startup, the
[shared model policy](../../../apps/backend/internal/agent/runtime/lifecycle/start_model.go)
requires exact catalog membership before calling `SetModel`.
It rejects the absent `sonnet` with `model_unavailable`,
`requested_not_advertised`, and `prompt_not_sent=true`. The current model
appears in the error as diagnostic evidence. The refusal follows
AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1.

The current managed bridge is 0.107.0, npm git head
`c5c120dba9c15bd88ea5d355fa3b77d1fc341392`, with Claude Agent SDK 0.3.323.
The published tarball integrity matched npm metadata.
Its preference resolver can match by family, and its ACP response supplies
selectable IDs and the current ID. The response supplies no alias binding.
Claude documents separate variables for alias mappings and initial selection.
[Pinned bridge source](https://github.com/agentclientprotocol/claude-agent-acp/blob/c5c120dba9c15bd88ea5d355fa3b77d1fc341392/src/session-model.ts)
[Claude model configuration](https://code.claude.com/docs/en/model-config)

## Scope

Implement the issue's save-time alternative. Check new and changed pairs of
recognized bare family aliases and different literal `ANTHROPIC_MODEL` values
in a strict Claude ACP profile. Keep the rejected draft visible and explain
how to select the full ID or remove the environment entry.

The check governs explicit profile inputs. Legacy pairs allow unrelated edits.
Full custom IDs and matching aliases follow existing save rules. Runtime
selection still uses the executor's catalog and applies the exact requested ID.
Authentication, workflow routing, and provider model resolution remain outside
this repair.

- [Requirements](../../specs/agents/requirements/no-silent-model-fallback.md)
  (AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.11 and .12)
- [Settings validation design](../../specs/agents/system-design/no-silent-model-fallback-01.md#claude-profile-save-validation)
- [Editor error design](../../specs/agents/system-design/no-silent-model-fallback-02.md#claude-save-error)
- [Decision](../../decisions/2026-11-20-claude-exact-model-alias-validation.md)

The agents system owns the saved model policy. The settings controller checks
the explicit pair at each settings write. The shared client action localizes
the validation error for the existing save error displays.

## ASCII UI preview

**UI-01 · Claude profile save error**

From Settings > Agents & Profiles, create or edit a Claude profile with the
reported settings and select Save.

```text
Current
  Start model          sonnet
  Fallback settings    Exact model required
  Environment Variables
    ANTHROPIC_MODEL    claude-sonnet-5-5
  [Save changes]       Saves the conflicting profile

After Save
  Start model          sonnet
  Fallback settings    Exact model required
  Environment Variables
    ANTHROPIC_MODEL    claude-sonnet-5-5
  [Save changes]

  Failed to save profile
  Choose a full model ID in Start model, or remove
  ANTHROPIC_MODEL from Environment Variables.
```

On a phone, the current settings route keeps the existing one-column
composition and Save control. The toast uses its existing viewport, wraps the
message, and leaves the draft and input controls reachable. The current
`AgentProfilePage` and the mobile agent profile settings flow supply the
layout. The error text and ability to retry are required. Spacing is
illustrative.

## Work order

[01. Validate the explicit pair at save and show the correction](task-01-validate-claude-model-pair.md)
covers the settings API, editor, and public guidance in one pass. It has no
delivery dependency. It includes exact Go, Vitest, desktop and phone
Playwright, localization, and docs commands.

## Diagnostic results

Local HEAD matched `main` at `de63958fc71a036e41021faded983035ec025189`.
[PR #5214](https://github.com/kdlbs/kandev/pull/5214) delivered the related
remote authentication fix. GitHub shows #5183 open and assigned to
`carlosflorencio`.

- `TestPlanningProbeClaudeAliasSaveRejected` called the real profile create
  handler with a fake repository. It failed as expected. The endpoint returned
  200, wrote one profile, and sent one create event for the reported settings.
- `TestPlanningProbeClaudeAliasBeforePrompt` used a mock agentctl session
  with current model and sole catalog entry `claude-sonnet-5-5`. The strict
  alias case produced the reported typed failure with zero model and prompt
  calls. The full ID case made one model call with the exact ID and delivered
  one mock prompt.
- The focused lifecycle controls in the work order passed on the baseline.
- The existing settings controls passed after review. They cover profile
  create, update, duplicate, provider fields, and strictness.

Both probes ran through temporary Go overlays and were cleaned up. They
reproduce settings admission and startup policy. The reporter's SSH run
provides the real executor evidence.

The spec catalog, spec lint, document links, and whitespace checks passed.
The PR documentation preflight accepted the work order references using
the planned backend and web source paths.

## Compatibility

The check can reject a newly configured pair even when an executor advertises
the alias. It requires a clear selection when the user supplies both inputs.
Profiles that get model settings from a secret or another environment layer
still rely on the runtime check. The accepted decision records the trade-off.
