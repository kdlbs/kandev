---
status: current
system: agents
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback: Recovery and Profile Surfaces

## Mapping and scope

This design completes [the policy design](no-silent-model-fallback-01.md).
REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001 acceptance criteria .6/.7 map to profile
surfaces, .8/.9 to recovery/workflow, and .2/.10 to warnings/advisories.

## Recovery and session ownership

Initial launch, context reset (`manager_interaction.go`), workspace rebind
(`manager_workspace_rebind.go`), and Kubernetes replacement
(`manager_kubernetes_refresh.go`) use the same resolved `StartModelPolicy`.
Preserve completed PR transport staging and delayed `session_models` handling.
A strict replacement waits for its own usable catalog, including an async
catalog after an empty session response. Timeout with no usable catalog fails
before inference. Never select from the previous published client on timeout.

A compatible replacement may continue after a bounded catalog wait when its
session initialized successfully but advertised no usable models. Pass only
replacement evidence (including absent/empty state) to the shared policy and
persist its warning. Distinguish catalog absence from initialization/stream
failure and context cancellation. Cancellation always stops the attempt.
An empty non-nil snapshot is not proof of catalog readiness. Keep replacement
publication, rollback, and conversation identity ordering already fixed by the PR.

In `sessionHasUnauthorizedExactModelDrift`, apply exact-model eligibility only
when the destination profile explicitly requires exactness. Dormant auto/fallback
values cannot authorize a different model for strict destinations. Unknown or
missing effective model also requires a fresh cross-step strict session.
Compatible destinations keep normal workflow reuse policy; model drift alone
is no longer a replacement reason. Profile changes and other existing session
eligibility rules still apply. Same-step explicit overrides remain intact.

Retain the validated candidate passed to the switch path and the one-lookup
empty-candidate fix. Profile-read errors do not silently downgrade strictness.
Test all callers of the shared drift check, including human-to-work transitions.
A new conversation must not inherit a rejected provider resume identity.

## Profile surfaces

Entry point: Settings > Agents > concrete agent profile > Fallback settings.
Also update the shared controls used by `components/agent/cli-profile-editor.tsx`.
The setting applies to that profile on any executor. There is no global,
workspace, or executor override and no added item under System Feature Toggles.

Extend `ModelFallbackSection` and `ModelFallbackSettingsShell`:

- Place Require exact model first, as a full-width row in the expanded section.
- Show a visible helper: "Stop before starting if the selected model cannot be
  used. Fallback settings do not apply while this is on."
- Strict on disables automatic and explicit fallback controls without clearing
  them. Strict off restores the old automatic/explicit interaction. A dormant
  auto=true must not win in the summary or control disabled state.
- Keep the disclosure initially collapsed, with effective summaries such as
  "Exact model required", "Automatic fallback", "Fallback: {model}", or
  "Executor default allowed". Summaries and helper copy must agree with the
  compatible selection/error matrix, including default continuation when an
  explicit fallback is also absent.
- Include the new field in dirty state, Save/Cancel, optimistic reconciliation,
  normalization, and reload hydration. Updating only strictness must not be
  mistaken for an enabled-only edit. Empty-model validation keeps the draft open.
- Keep the saved start model unchanged. An unavailable host model does not
  disable the strict switch if the profile already has a concrete saved ID.
- Use `t()` and settings namespace keys for all copy. Translate English,
  Portuguese, Simplified Chinese, and generated Traditional Chinese catalogs;
  regenerate pseudo. Do not publish copy that describes every auto=false profile
  as strict or promises a configured fallback is always the final alternative.

## Mobile contract

Nearest shipped exemplar: `ModelFallbackSettingsShell` with `FallbackOptionHelp`.
Reuse its shared form state, collapsed disclosure, and `useTouchDrawer` help.
Desktop keeps the strict row above two fallback columns. Phone uses one ordered
vertical flow: strictness, automatic fallback, explicit fallback, existing Save.
No separate phone policy or copied business logic.

This is an occasional settings edit; it remains inline on the profile page.
Use an inset bottom drawer only for supplementary help. The existing page or
editor scroll container remains the sole form scroll owner. Shared drawer
primitives handle dynamic viewport height and safe-area spacing. Close help
returns focus to its trigger without closing the form or losing draft values.

Ordinary desktop controls keep 28px sizing; touch row/help hit targets are at
least 44px (both dimensions for icon buttons), scoped to phone/coarse pointers.
Measure actual bounds and hit testing. Check long translated labels, disabled
states, keyboard toggle/save, document overflow, and coarse-pointer tablets.
The plan's UI-01/UI-02 previews define order and grouping, not pixel geometry.

## Verification and remaining limits

Use real store migrations for upgrade evidence, policy unit tests for the full
matrix, and lifecycle transport tests for delayed replacement catalogs and
cancellation. Pair model-policy tests with an actual migrated profile launch;
a fabricated zero-value policy alone does not prove upgrade compatibility.
Desktop and mobile E2E cover saved toggles, launch continuation/error, drawer
interaction, and persisted warnings. A mock E2E catalog mismatch does not prove
provider credentials or every external executor integration.

Keep public docs updates in the implementation work orders. The existing public
pages now describe the implemented compatible-by-default policy.
