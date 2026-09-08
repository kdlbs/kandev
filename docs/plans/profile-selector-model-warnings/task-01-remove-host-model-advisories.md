---
id: "01-remove-host-model-advisories"
title: "Remove host model advisories from profile selectors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-003
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.4
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.7
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.2
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.4
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-003.6
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
---

# Task 01: Remove host model advisories from profile selectors

## Summary

Remove the host-catalog warning from profile options and their selected labels.
Keep agent health indicators, editor diagnostics, and actual model-selection
warnings. Deliver the change through the shared hook with focused regression
tests and updated public guidance.

## In scope

- Own the shared hook, associated component tests, selector-copy cleanup, and
  existing desktop/mobile model-selection E2E files.
- Add a successful exact-model launch with differing host and executor catalogs,
  using disposable mock profiles and existing catalog fixtures.
- Keep the existing actual-fallback browser tests as preservation checks.
- Update the agent-profile guide's explanation of warning placement.
- Update this work order and the plan's results after the exact checks pass.

## Out of scope

- Production Go changes, ACP normalization, discovery cache/polling, schema,
  migrations, model-selection policy, or runtime warning metadata.
- Caller-specific warning flags, new overlays, new settings, or profile rewrites.
- Broad verification and repository-wide review. The user's later `@implement`
  request separately authorized commit, push, PR creation, and PR fixup.

## Acceptance

1. Both label render paths pass the plan's host-catalog matrix without model
   advisories. Existing health indicators, eligibility, ordering, and saved
   profile configuration remain intact.
2. Desktop keyboard and mobile touch flows select and launch the requested
   profile. A successful exact-model launch has no model-selection warning;
   actual fallback still displays one persisted warning after reload.
3. Unused selector translations are removed consistently. The profile editor
   keeps its advisories and the public guide explains the shipped behavior.

## Implementation sequence

1. Read the linked requirements and design section 8, scoped web guidance,
   existing source, and test helpers. Mark this work order in progress only
   after the user requests implementation.
2. Run the new component absence assertions against the current implementation
   and record their expected failure. Cover both render paths, not only the old
   warning-button test ID.
3. Update the desktop/mobile selector scenarios for the intended behavior and
   run the focused files against the current production build to record red.
   Keep actual-fallback tests intact.
4. Remove the host model comparison, warning renderers, and exclusive imports.
   Preserve the hook interface, health warning renderer, and caller data loading.
5. Complete the successful executor-model fixture, saved-value checks, and
   localization cleanup. Update public guidance and run the commands below.
6. Mark done only after targeted checks pass. Record outcomes and any remaining
   limitations in this file and the manifest.

## Verification

Run from the repository root. Install once in a fresh worktree:

```bash
rtk pnpm --dir apps install --frozen-lockfile
```

Use the first command for red and green component checks. The listed preservation
suites cover editor and chat behavior without a broad audit:

```bash
rtk pnpm --dir apps/web exec vitest run components/task-create-dialog-options.test.tsx components/settings/profile-form-fields.test.tsx lib/model-variation.test.ts components/task/chat/messages/status-message.test.tsx
rtk pnpm --dir apps/web exec eslint components/task-create-dialog-options.tsx components/task-create-dialog-options.test.tsx
rtk pnpm --dir apps/web run typecheck
rtk pnpm --dir apps/web run i18n:check
rtk pnpm --dir apps/web run i18n:ratchet
```

Run projects sequentially. The managed runner builds production assets and
isolates its instance. Check discovery counts; zero tests is not a pass.

```bash
rtk pnpm --dir apps/web e2e:run --project chromium tests/settings/no-silent-model-fallback.spec.ts tests/session/model-mismatch-warning.spec.ts
rtk pnpm --dir apps/web e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts tests/session/mobile-model-mismatch-warning.spec.ts
```

Inspect the rendered mobile selection state or its test screenshot for row
spacing, viewport containment, and the absence of an empty warning hit area.
Tests must verify selection completion and no document horizontal overflow.

After public-documentation changes:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

Audit removed controls and keys:

```bash
rtk rg -n 'agent-profile-model-probe-warning|profileStartModelNotAdvertisedOnHost|profileStartModelUniqueVariationOnHost' apps/web
```

Only intentional negative assertions may retain the removed control identity.
Production references to the two translation keys must be absent. Keep
`modelVariationAdvisory` and existing runtime-warning keys.

## Files likely touched

- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/task-create-dialog-options.test.tsx`
- `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/profile-model-selection-helpers.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning-helpers.ts`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/settings.json`
- `docs/public/agents-and-profiles.md`

Preservation inputs, normally unchanged:

- `apps/web/lib/capability-warning.ts`
- `apps/web/components/settings/profile-model-fields.tsx`
- `apps/web/components/settings/profile-form-fields.test.tsx`
- `apps/web/lib/model-variation.ts` and its test file
- `apps/web/components/task/chat/messages/status-message.test.tsx`
- `apps/web/e2e/tests/session/model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/mobile-model-mismatch-warning.spec.ts`
- The six shared-hook consumers listed in the manifest

## Dependencies

None.

## Risks

The selected-label path can retain a warning after the option path is fixed.
Blanket icon removal can hide capability errors. Fixture setup must prove
different host and executor catalogs, and cleanup must delete only the
disposable profiles created by the tests.

If removing the host-catalog subscription exposes a caller that relies on it
for capability loading, retain that loading through the caller's existing
domain hook. Do not introduce another model-warning calculation.

## Parallelism

`sequential`. No subagents are authorized.

## Inputs

- [Requirements](../../specs/agents/requirements/no-silent-model-fallback.md):
  001.4–001.5, 002.7, and 003.
- [Design part 1](../../specs/agents/system-design/no-silent-model-fallback-01.md):
  section 8, runtime warning contract, and tests.
- [Design part 2](../../specs/agents/system-design/no-silent-model-fallback-02.md):
  warning-removal and evidence risks.
- [Executor authority ADR](../../decisions/2026-08-15-executor-authoritative-model-selection.md).
- [Variation resolution ADR](../../decisions/2026-09-07-unique-model-variation-resolution.md).
- Existing option/selected-label renderers and their component harness.
- Existing model-mismatch helpers and mock catalog environment fixture.
- Mobile exemplar and composition rationale in design section 8.

## Results

Completed on 2026-09-08.

- Removed host-catalog membership checks and both model-warning render paths
  from the shared hook. Capability health, eligibility, and recent-use behavior
  remain unchanged. Existing callers retain their domain-level capability loading.
- Removed the two selector-only keys from all six locale catalogs. Editor
  diagnostics and runtime-warning code remain unchanged.
- The red component run failed 11 of 23 assertions. The old production build
  failed both selector cases in each browser project on the warning controls.
- The four targeted unit suites passed all 55 tests.
- The managed desktop run passed six tests with `--host --project chromium`.
  The mobile run passed four tests with `--host --no-build --project mobile-chrome`.
  Both used the exact file lists in Verification. The mobile run reused the
  unchanged production assets from the desktop run.
- The successful-launch fixture proves that the host omits `opus[270k]` and the
  executor advertises it. It verifies the model, effort, zero warnings, and
  unchanged saved profile after completion and reload.
- The first mobile launch run failed because it assumed desktop navigation.
  The corrected test taps the created task card before it checks the session.
- Targeted ESLint passed for the component, its test, and all four changed or
  new E2E files. Typecheck, `i18n:check`, and `i18n:ratchet` passed.
- Public-documentation validation passed: 61 validator tests and 46 pages.
- Fresh desktop/mobile screenshots show both the option list and selected label.
  The mobile screenshots show usable row spacing with no empty warning control.

No backend changes, schema changes, live-instance restarts, or profile rewrites
were required. The screenshot-time browser catalog remains unknown.
