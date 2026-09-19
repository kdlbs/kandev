---
status: active
system: agents
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback Requirements

## Overview

Profiles retain their model choices when executor capabilities differ from the
host. Users can require exact selection for an individual profile. Upgrading
Kandev must not turn existing fallback behavior into strict launch failures.

The agent system owns this contract because it owns persisted profile policy.
Task and Office execution consume that policy. This specification amends
PR #3473 and is implemented by the profile policy and recovery designs.

## Terminology

- **Strict profile:** a profile with Require exact model explicitly enabled.
- **Compatible profile:** a profile with Require exact model disabled or absent.
- **Requested model:** the effective model for the launch, including an
  intentional session override where the existing session contract permits it.
- **Fallback:** a different model selected or retained for the current launch.

## Requirements

### REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001: Explicit strictness and visible fallback

**Intent:** Prevent unwanted substitutions when explicitly requested, while
preserving working executor launches for existing profiles.

- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1:** When Require exact model is
  enabled, the executor shall advertise and apply the requested model before
  the initial prompt. Otherwise the attempt shall fail before inference.
  A fallback setting or advertised variation shall not bypass this rule.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.2:** When a compatible profile uses
  a different model, the session shall show one durable warning for that
  selection decision. It shall include the requested and effective model when known.
  Reload and event replay shall not duplicate the warning.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.3:** Existing profiles and newly
  created profiles shall have Require exact model off unless explicitly set.
  An upgrade shall preserve the model, fallback model, automatic fallback,
  and compatible launch outcomes. An absent setting shall not imply strictness.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.4:** For compatible profiles, an
  advertised requested model shall take priority. With automatic fallback off,
  an advertised explicit fallback shall take priority over variation matching.
  If no alternate selection is available, the session shall continue on the
  executor current/default model with a warning. An empty catalog or unsupported
  model-selection method shall also permit this continuation.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.5:** Compatible profiles shall retain
  existing apply-error behavior. An ordinary error applying an advertised model
  shall fail unless automatic fallback is enabled. Automatic fallback shall
  continue with a warning and ignore the configured explicit fallback.
  Initialization failures, disconnections, and cancellation shall remain errors.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6:** Require exact model shall be a
  per-profile setting in Fallback settings, available on desktop and mobile.
  It shall default off. Enabling it shall disable the fallback controls without
  erasing their values. Disabling it shall restore their previous behavior.
  Save, reload, duplication, and supported profile export/import shall preserve
  the setting. Omitted fields in partial updates shall preserve a saved choice.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.7:** A collapsed Fallback settings
  section shall summarize the effective policy and expose unsaved changes.
  Strictness shall be keyboard and touch operable with visible explanatory copy.
  Phone help shall use the existing drawer interaction, with reachable touch
  targets, focus return, and no horizontal document overflow.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.8:** Launch, reset, fresh workspace
  rebind, and replacement-executor recovery shall apply the same profile policy.
  Replacement selection shall use only replacement-session evidence. Cancellation
  shall stop the attempt. A strict attempt without a ready replacement catalog
  shall fail; an initialized compatible attempt may continue with a warning.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.9:** Across different workflow steps,
  an explicitly strict destination shall replace a session whose effective model
  is different or unknown. Compatible destinations shall not force replacement
  solely because the effective model differs. Same-step intentional overrides
  shall keep their existing behavior. A validated empty reuse lookup shall create
  a fresh session without an unvalidated second lookup.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.10:** A missing host-probe model shall
  remain an advisory and shall not disable profile selection. Runtime selection
  shall not send an unadvertised model or rewrite the saved profile model.
  Enabling strictness without a concrete model shall produce a validation error.

### REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002: Compatible advertised variation selection

**Intent:** Preserve the pre-PR compatible selection order without treating a
variation as an exact identity. This amendment restores the scoped behavior
removed by the strict-by-default PR.

- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1:** This rule applies when strictness
  and automatic fallback are off. The bare requested ID must be absent, and no
  advertised explicit fallback must be available. Under these conditions, exactly
  one distinct advertised bracketed variation shall be selectable with a warning. Zero or multiple matches, malformed
  variations, and already-bracketed requests shall not authorize a guessed ID;
  the session shall use the executor default with a warning. Saved IDs shall
  remain unchanged. Strict profiles shall never infer a variation.

## Exclusions

No global, workspace, executor, environment, or release-toggle setting. No
provider switching caused by this new field. Office post-start routing remains
workspace-owned. No retroactive interruption or model change of running turns.
No guarantee that invalid credentials, missing executables, or network failures
can launch successfully. No new global enforcement system for model costs.

## System design and delivery

- [Policy and persistence](../system-design/no-silent-model-fallback-01.md)
- [Recovery and profile surfaces](../system-design/no-silent-model-fallback-02.md)
- [PR implementation package](../../../plans/exact-profile-model-identity/plan.md)
