---
id: "07-combined-acceptance"
title: "Prove both issues end to end"
status: in_progress
wave: 7
depends_on:
  - "06-selector-integration"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-001.2
  - AC-AGENTS-PROFILE-DISCOVERY-003.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.5
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 07: Prove both issues end to end

## Summary

Prove the complete first-profile and grouped/composed selection outcomes on desktop and phone.
Use deterministic ACP fixtures and real save/application boundaries, with no paid account or model inference required in CI.

## In scope

- A shared grouped/flat fixture with qualified Fusion bindings and a differently named synthetic provider.
- First Devin draft without a parent, profile/workflow save, live Apply, reload/restore, stale results, and partial failure.
- Phone Back/focus, one-sheet behavior, touch targets, containment, safe-area/internal scrolling, and viewport changes.
- Public model-selection documentation and final cross-package results.

## Out of scope

- Another generic QA/review pass, production provider inference, fabricated Fusion bindings, or pricing/recommendation badges.

## Acceptance

1. Both issues' complete outcomes pass on desktop and phone, including verified Fusion selection and the synthetic second provider.
2. Partial errors, cancellation, stale responses, and restoration preserve actual owning state and conversation identity.
3. The work-order results contain actual commands/counts, public docs, and an explicitly satisfied Fusion qualification gate.

## ASCII UI preview

Use all [UI-GROUP views](plan.md#ascii-ui-preview), including the failure and phone states.

```text
Desktop: Models -> Group -> exact dimensions/options -> Use configuration
Phone:   One sheet -> body choice list -> Back -> Use configuration
Settings: Apply -> current draft -> existing Save
Live:     Confirmed model/options, or visible partial failure
```

These are interaction requirements, not a second visual design.
Assert one active picker/sheet and keep focus return tied to its trigger.

## Fixture and scenario matrix

Reuse the disposable Devin descriptor from the permission companion and extend the existing mock ACP peer with grouped catalog scenarios.
Read `apps/backend/cmd/mock-agent/AGENTS.md` before editing its fixtures and rebuild through the managed runner.
The fixture must exercise native ACP grouping and backend normalization, rather than fulfill successful HTTP probes in Playwright.
Use Task 04's qualified real shape for Devin bindings, with synthetic public labels/data where privacy requires it.
Use a second generic composition with different dimension names and opaque exact model values.

Cover these cases on both viewport projects:

- Missing-parent first Devin draft: discover groups, configure a valid pairing/options, Save, and reload the persisted exact values.
- Existing profile and workflow override: navigate/search, configure, Apply to draft, Save, and reopen.
- Live session: preserve conversation identity across ordinary and composed model changes and later configuration changes.
- Partial Apply: model succeeds, option fails, confirmed state remains visible, retry starts from that state.
- Race: resolve A, choose B, then deliver A late. Only B's matching options remain authoritative.
- Cancel and resize: unfinished dimensions remain during navigation/viewport changes and never apply on dismissal.
- Restore: refresh and reset/resume retain selected values, baseline summaries, and prior turn attribution.
- Compatibility: flat provider, unknown binding, unsupported option, repeated labels, and the synthetic provider.

Phone tests use touch input and assert at least 44px targets, internal Back, focus return, viewport containment, and no document horizontal overflow.
Assert safe-area clearance and internal scrolling with content tall enough to exercise them.
Restore owned rows, worker environment overrides, and fixture files in cleanup, including failure paths.

## Verification

Run from the repository root after the frozen dependency install, if needed.
Keep managed E2E commands sequential and use their resource guardrails.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/chat/grouped-model-selector.spec.ts tests/settings/grouped-profile-workflow-models.spec.ts tests/chat/model-selector-consecutive-switch.spec.ts tests/settings/first-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-grouped-model-selector.spec.ts tests/settings/mobile-grouped-profile-workflow-models.spec.ts tests/chat/mobile-model-selector.spec.ts tests/settings/mobile-first-profile-capability-discovery.spec.ts tests/settings/mobile-agent-profile-config-selector.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/first-draft-profile-discovery docs/plans/devin-profile-permissions
git status --short -- docs/plans/first-draft-profile-discovery docs/plans/devin-profile-permissions
```

Add every changed E2E helper/spec to an explicit ESLint command and record it in Results.
Record the managed runner's actual discovered/pass counts by project.
Run the PR documentation coverage preflight with all linked plans/work orders/designs before final handoff.

## Files likely touched

- New `apps/web/e2e/tests/chat/grouped-model-selector.spec.ts` and `mobile-grouped-model-selector.spec.ts`
- New `apps/web/e2e/tests/settings/grouped-profile-workflow-models.spec.ts` and `mobile-grouped-profile-workflow-models.spec.ts`
- New `apps/web/e2e/helpers/grouped-model-catalog.ts`
- Existing mock ACP peer with deterministic grouped/composition/update scenarios
- Existing first-profile and consecutive-switch E2E fixtures where integration requires them
- `docs/public/agents-and-profiles.md` and `docs/public/tasks-and-workflows.md`
- Results/status fields in both linked plan packages

## Dependencies

Tasks 01 through 06 and both permission work orders.
Task 04 must contain verified real Fusion bindings. A synthetic-only mapping is insufficient.

## Risks

- An HTTP-fulfilled happy path can conceal converter loss or scope bugs.
- A short phone fixture cannot prove the sheet's scroll and safe-area behavior.
- A generic grouped-only pass cannot close the real Fusion composition acceptance.

## Parallelism

`sequential`

## Inputs

- [Combined plan](plan.md)
- [Grouped catalog design](../../specs/agents/system-design/grouped-model-catalog.md)
- [First-profile design](../../specs/agents/system-design/profile-capability-discovery.md)
- Task 04 qualification record and the existing mobile/model-switch/profile-discovery E2E patterns.

## Results

Partial, 2026-11-03.

Delivered and verified:

- Issue #5023: the desktop and phone first-profile specs pass against the real backend (`probe` and `resolve`), including an omitted `profile_id`, a complete launch snapshot, dependent option resolution, Save, and reload.
- Grouped/composed catalogs: backend round trips, hydration, selector rendering, search, exact composition resolution, invalid/ambiguous rejection, partial-application semantics, and phone sheet behavior are covered by unit, component, and lifecycle tests.
- Public documentation was updated in `docs/public/agents-and-profiles.md` (first-profile discovery, groups/compositions, Devin permissions); `node scripts/validate-public-docs.mjs` validated 54 pages.
- The Devin permission defaults, validation, and settings control are implemented and localized.

Not verified or not delivered:

- At the original handoff, Task 04 had no machine-readable Lead/Sidekick source or configured Devin executable. Verified Fusion acceptance for #5026 was **not met**. The updated [qualification record](fusion-qualification.md) now identifies reported native JSON evidence, with capture and implementation still pending.
- No browser fixture supplies a catalog with compositions. Composed Apply and live partial application are covered by component/integration tests, not Playwright.
- The mock ACP peer does not yet expose native grouped catalogs, so grouped browser navigation has no deterministic fixture.
- The grouped-catalog E2E files named in this work order were not added; the first-profile and Devin permission specs are the new browser coverage.
- The Devin capability scenarios (Smart present, Smart absent, sandbox initialization failure) are not exposed by the mock peer.

## Follow-up execution for issue #5053

The [selector repair browser work order](../model-catalog-selector-repair/task-04-browser-acceptance.md) owns the remaining grouped/Fusion browser matrix.
It uses the four browser filenames above and adds explicit singleton, mixed fallback, and contextual native acquisition evidence.
Its native fixture gate remains separate from deterministic browser results.
Keep this work order `in_progress` until the complete remaining model-selection acceptance passes.
The earlier command counts remain historical. Append new actual results after implementation.
The permission capability scenarios remain a distinct companion-package gap and are not part of the selector repair.
