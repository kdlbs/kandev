---
created: 2026-10-08
status: draft
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
  - REQ-PLATFORM-TURN-CONTINUITY-003
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
legacy_specs: []
---

# Implementation plan: Unconditional interruption continuation

## Overview

Remove the installation opt-in for supported interruption recovery. First retire
the flag throughout configuration, agentctl evidence, admission, dispatch, and
frontend feature state. Then prove ordinary desktop and phone recovery without
an enabling override and update operator guidance. Execute both work orders
sequentially; implementation and product verification remain pending.

Platform owns the recovery policy and retired runtime identity. Agents retains
provider capability translation, and Tasks retains prompt and turn ownership.
The [graduation decision](../../decisions/2026-10-08-unconditional-interruption-continuation.md)
records the user's choice to remove the gate rather than promote its default.

## Evidence

The reported task `69b1f974-3b49-4792-ad2f-8365afb4b355` and session
`071361f2-1936-4bc7-a268-97f698bbe4f0` failed on October 8 at 11:56:43 Lisbon
time. The backend logged refusal without safe prompt-attempt evidence. The saved
failure has `failure_code=agent_transport_lost`, `attempts_started=0`,
`recovery_disposition=refused`, and `runtime_retained=true`. The runtime registry
reports continuation off with no saved override. The completed work precludes
original-prompt replay. An active long-running shell call was cancelled at prompt
settlement; retirement does not establish safe continuation for that call.

Current gates exist in backend composition, `continuationFailureHasEvidence`,
`validateContinuationOwner`, ACP evidence/permission tracking, and the full
managed-agentctl startup configuration chain. Changing only `profiles.yaml`
would leave a live flag and stale overrides capable of disabling recovery.

## Scope

### In scope

- Remove the active flag and its propagated booleans, including standalone defaults.
- Retire the exact key/environment identities while preserving inert overrides.
- Collect supported prompt safety evidence and admit safe continuation in all profiles.
- Keep existing provider, ownership, tool-outcome, native-identity, and retry boundaries.
- Prove default recovery, old false values, settings omission, and desktop/phone feedback.
- Reconcile public config, feature-status, and session recovery guidance.

### Out of scope

- Automatic recovery after uncertain or cancelled tool work.
- Original-prompt replay after output/tools, new provider support, or changed retry limits.
- New feature flags, database migrations, schedulers, or persistent retry jobs.
- Replaying old failures, repairing the reported conversation, or changing the live instance.
- Changes to the separate initial-task-brief-after-recovery package.

## Technical approach

### Retirement and configuration

Remove the profile entry through root `profiles.yaml` (the embedded source is
`apps/backend/internal/profiles/profiles.yaml`). Remove the `FeaturesConfig`
field, active registry registration, and frontend `defaultFeatureFlags` property.
Append the pair to existing `retiredRuntimeFlagIdentities`; completeness/collision
tests already exist. Stored overrides remain unchanged and inert. Preserve other
flags' environment/override/profile precedence.

Delete `ProviderInterruptionContinuation` from `orchestrator.Config`, backend
composition, `AgentctlStartupConfig`, server/instance configs, process adapter
config, `adapter.Config`, and shared transport config. Do not keep an always-true
boolean or introduce an environment fallback. Cover managed startup and direct
standalone startup; ignored legacy JSON and stale YAML/environment/override
values cannot turn off the supported behavior.

### Evidence and recovery

Remove only the flag terms in `adapter_continuation.go`,
`adapter_continuation_permissions.go`, `continuationFailureHasEvidence`,
`continuationRefusalReason`, and `validateContinuationOwner`. Retain dialect and
native restore support, non-loading session identity, prompt generation, immutable
terminal evidence, permission ownership, foreground completion, and the 256-tool
bound. Preserve V1/V2 semantics and refusal for missing/unknown evidence.

Keep `handleTransientFailure`, its five-attempt owner, hidden exact `continue`,
retained-runtime reuse, strict native restore, and original replay fence. Keep
capacity continuation's separate admission policy. Existing persisted
`recovery_reason=disabled` records may retain their historical renderer and locale
keys; no active backend path emits that reason after graduation.

### Compatibility

| Provider or shape | Result | Required evidence and fallback |
| --- | --- | --- |
| Cursor ACP V2, same native ID, output or all tools completed | Normal internal continuation | Default-config ACP and orchestration tests plus desktop/phone mock wire traces |
| Cursor ACP V1, valid original read-only snapshot | Existing limited continuation | Preserve V1 admission/dispatch regressions |
| Unsupported dialect, missing restore capability, old helper without attestation | Manual/existing policy | Capability, omission, unknown-version, and identity negatives |
| Pending, cancelled, failed, conflicting tool work or unresolved permission | Manual recovery | Mixed-completion and permission ownership negatives |
| Concrete capacity-live policy | Existing live-runtime continuation | Existing capacity regressions, independent of transport-native evidence |
| Dynamic, Office, utility, passthrough | Existing owner/policy | Existing excluded-context negatives; no newly inferred capability |
| Former config key/env/override=false | Inert legacy input | Actual config/store/startup tests; recovery stays eligible |

Completed native Cursor evidence in the prior package remains applicable because
this package changes availability, not the native wire contract. Mock tests do
not claim new live-provider proof. If implementation changes native restoration
or payload semantics, stop and define the additional native verification first.

## ASCII UI preview

```text
UI-01: Settings > System > Feature Toggles (desktop and phone)
  Other active feature toggles
  [No Interrupted conversation continuation row]

UI-02: Task Chat after an eligible interruption, no opt-in
Desktop
  User request and completed tool history
  Cursor: Attempt 1/5. Continuing in 5s.   [Cancel]
  Cursor: Continuing...
  Agent output completing the work

Phone, full-height Chat
  User request and completed tool history
  Cursor: Attempt 1/5
  Continuing in 5s.
  [ Cancel: at least 44px ]
  Agent output completing the work
  Composer in existing safe-area region
```

Entry is the existing task Chat destination. `TransientRetryNotice` and the
shipped phone continuation spec are the exemplars. Chat remains the single
transcript scroll owner; desktop density, phone stacked actions, technical
wrapping, focus, and safe areas remain intact. No automatic `continue` user bubble
appears. UI-01 maps to `002.6`; UI-02 maps to `001.6` and `003.1` through `003.4`.
Wording is illustrative and reuses localized copy; structural conditions are
required. No new layout or interaction primitive is needed.

## Tests

| Criteria | Coverage |
| --- | --- |
| `002.5`, `002.6` | New `TestInterruptionContinuationGraduatedFlagContract`; stale override and env/config load; registry/profile/frontend equality |
| `001.1`, `001.2`, `001.5`, `002.5` | New `TestCursorContinuationEvidenceWithoutOptIn`; existing completed/uncertain outcomes, permissions, prompt-gate handoff, generation, and wire-version tests |
| `001.1` through `001.7`, `002.1` through `002.4` | New `TestInterruptionContinuationAdmissionWithoutOptIn`; existing continuation, retained-runtime, replay safety, budget, identity, queue, cancellation, and restart tests |
| `003.2`, `003.3` | Retain historical failure rendering and lifecycle cleanup tests |
| Turn continuity `003` | Existing capacity continuation tests; no transport toggle fixture needed |

Test names marked new are work-order deliverables, not executed tests. All
unqualified suffixes refer to `AC-PLATFORM-INTERRUPTION-CONTINUATION`.

## E2E tests

Update `e2e/helpers/provider-interruption-continuation.ts` to stop forcing the
flag on. Keep the full desktop continuation scenario matrix in
`tests/session/provider-interruption-continuation.spec.ts` under `chromium`.
Replace its disabled-installation case with a former-env=false inertness case;
prove registry/features omission and actual native prompt traces.

Run `tests/session/mobile-provider-interruption-continuation.spec.ts` under
`mobile-chrome` without a flag override. Add a completed-foreground-tool default
case on phone alongside the existing Cancel/reload case. Assert the same native
ID, one original prompt, hidden continuation, preserved history, 44px Cancel,
and no horizontal overflow. Keep uncertain-work negatives, budget exhaustion,
queued-human priority, and both backend-restart/agent-survival cases. Run the
managed desktop and phone commands sequentially.

## Work orders

- [ ] [Task 01: Retire the continuation toggle](task-01-retire-continuation-toggle.md)
- [ ] [Task 02: Prove default recovery and update guidance](task-02-default-recovery-proof.md), after Task 01

## Related packages and public docs

Completed `provider-interruption-continuation`, `cursor-hidden-continuation`,
`cursor-retriable-stream-reset`, and `transient-turn-runtime-continuity` packages
retain their historical scopes and results. This package owns new default and
retirement evidence; it does not rewrite their successful test counts.

After implementation, use `/docs-maintainer` to update
`docs/public/sessions-and-review.md` (explanation), `configuration.md` (reference),
and `feature-status.md` (reference). Remove opt-in/restart instructions for this
retired flag and state the normal eligibility/manual-recovery boundary. Preserve
existing public section anchors or redirect them. Search README and screenshot
guidance for stale availability wording. No website publication is part of this
package.

## Verification results

Implementation and product checks are pending. This design turn changes durable
documents only; no production or permanent test edits were made.

Design validation on 2026-10-08:

- `python3 scripts/list-docs.py validate`: passed, 364 decisions and 1437 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs:validateCoverage`: the docs-only snapshot is exempt;
  the prospective backend/ACP/frontend/E2E paths are covered by valid work orders.
  This is structural planning evidence, not verification of implementation.
- `git diff --check`: passed. The six pre-existing initial-task-brief files were
  preserved as a separate pending package.

Record exact execution results here after implementation; do not treat earlier
package results as this package's product tests.

## Risks

- Removing only profile defaults leaves stale overrides or helper booleans active.
- Collecting evidence by default must remain bounded and scoped to supported prompts.
- Existing tests that use the disabled flag as their only safety boundary need actual capability/outcome negatives.
- Old remote helpers remain manual until they supply recognized evidence.
- The reported cancelled shell call is not made automatically recoverable by this change.
