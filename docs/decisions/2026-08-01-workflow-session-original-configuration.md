# ADR-2026-08-01-workflow-session-original-configuration: Keep Workflow Restoration Separate From Provider Defaults

**Status:** accepted
**Date:** 2026-08-01
**Area:** backend, frontend, workflow

## Context

Workflow steps need to change an existing ACP session's model and select options and later restore the values with which that task session originally started. The existing ACP provider-default baseline is intentionally comparison-only: it is captured before profile and runtime overrides, so it cannot restore a profile-selected model while also restoring provider defaults for unspecified options. Profile rows and the mutable session snapshot are also unsuitable because either may change after the session starts.

## Decision

Each original task session records a separate, immutable original effective configuration after provider defaults and agent-profile selections settle but before workflow session-setting rules apply. The snapshot contains the effective model and every advertised selectable ACP option's raw value. Workflow `restore_original` rules are the only behavior that reads this snapshot to mutate provider state.

The existing provider-default baseline remains comparison-only. Mutable provider state remains in `runtime_config`, and explicit user/workflow selections remain in `runtime_config_overrides`. A successful restore writes the captured original values back as explicit runtime overrides so resume behavior does not depend on later provider-default or profile changes.

Workflow rules match the original session's stable agent name and never activate a different session. Sessions that predate the immutable snapshot are not reconstructed from mutable data; restoration fails visibly and leaves them unchanged when no trustworthy snapshot exists.

## Consequences

- Restoration represents what the task actually started with, including profile-selected values and provider defaults the profile left untouched.
- Compact changed-value summaries keep their existing provider-default comparison semantics.
- Session metadata gains another intentionally immutable configuration snapshot with a distinct lifecycle from display baselines, live state, and overrides.
- New-session initialization must preserve ordering so the original effective snapshot is captured before start-step workflow overrides.
- Legacy sessions may carry settings forward but cannot always restore automatically.
- Provider or profile changes after task creation do not redefine that task's original settings.

## Alternatives Considered

- Reuse the provider-default baseline: rejected because it omits the effective profile layer and could restore the wrong model.
- Re-read the current agent profile during restore: rejected because profile edits would retroactively redefine a running task's original settings.
- Use the mutable agent-profile or runtime snapshot: rejected because model changes and provider events update those values during the session.
- Clear runtime overrides and let launch defaults re-resolve: rejected because it would not actively set default-valued options and later provider/profile changes could alter the result.
- Reconstruct legacy originals heuristically: rejected because a plausible but incorrect model or effort setting is worse than a visible no-op.
