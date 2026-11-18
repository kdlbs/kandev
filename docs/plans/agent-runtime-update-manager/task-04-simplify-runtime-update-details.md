---
id: "04-simplify-runtime-update-details"
title: "Simplify runtime update details"
status: done
wave: 4
depends_on:
  - "02-runtime-update-manager"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-MANAGER-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-MANAGER-001.3
  - AC-AGENTS-RUNTIME-MANAGER-001.4
  - AC-AGENTS-RUNTIME-MANAGER-001.5
  - AC-AGENTS-RUNTIME-MANAGER-001.6
  - AC-AGENTS-RUNTIME-MANAGER-001.7
  - AC-AGENTS-RUNTIME-MANAGER-001.8
system_design:
  - ../../specs/agents/system-design/runtime-update-manager.md
---

# Task 04: Simplify runtime update details

## Summary

Simplify the individual runtime update dialog and phone drawer around the selected version and one primary update action. Keep technical information, commands, guidance, and completed output available on demand while preserving existing policy and update behavior.

## In scope

- Show the selected and current versions, one searchable Change version action, automatic-update setting, and an action that names the selected target.
- Move package, ownership, command preview, manual guidance, and successful output into collapsed Runtime details.
- Keep the successful result visible while allowing a new pinned-version review; self-update flows keep their targetless action and no version picker.
- Keep the same content inset across the desktop body, footer, and details. Preserve phone containment and touch targets.
- Retain explicit policy Save and Reset, update lifecycle states, operation-specific actions, focus return, and localized copy.
- Update the runtime detail requirement, design, plan preview, and public guide.

## Out of scope

- Changing update admission, API contracts, version validation, policy persistence, or defaults.
- Removing the searchable stable-version history, Kandev-default choice, manual guidance, or operation-specific recovery actions.
- Changing runtime ownership, backend update jobs, notifications, or unrelated Settings controls.

## Acceptance

1. Version-managed details show selected and current versions, one version picker, automatic-update setting, and the selected-target action in the primary view.
2. Runtime details start collapsed and expose package and owner information, command preview, applicable manual guidance, and retained successful output when expanded.
3. Existing policy drafts and update actions preserve their prior save, admission, progress, completion, and recovery behavior. A completed pinned update keeps its result visible until a new version review starts.
4. Desktop sections share one inset. The phone drawer keeps controls reachable, contains the picker, and retains at least 44px touch targets.

## ASCII UI preview

See [UI-03: Runtime detail and dirty policy](plan.md#ui-03-runtime-detail-and-dirty-policy), which covers AC-AGENTS-RUNTIME-MANAGER-001.3 through 001.8.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/settings/agent-runtime-update-control.test.tsx components/settings/agent-runtime-policies.test.tsx components/settings/agent-runtime-update-manager.test.tsx hooks/domains/settings/use-agent-runtime-update-manager.test.tsx)
(cd apps/web && pnpm exec vitest run lib/dialog-default-action.test.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/settings/agent-runtime-update.spec.ts tests/settings/agent-runtime-notifications.spec.ts tests/settings/agent-runtime-update-copilot.spec.ts tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-agent-runtime-update.spec.ts tests/settings/mobile-agent-runtime-notifications.spec.ts tests/settings/mobile-agent-runtime-update-copilot.spec.ts tests/settings/mobile-agent-self-update.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm exec tsrs --noEmit --fast)
(cd apps/web && pnpm run i18n:check)
```

Specification and documentation checks are recorded in the plan's verification results.

## Files likely touched

- `apps/web/components/settings/agent-runtime-update-content.tsx`
- `apps/web/components/settings/agent-runtime-update-surface.tsx`
- `apps/web/components/settings/agent-runtime-update-technical-details.tsx`
- `apps/web/components/settings/runtime-version-picker.tsx`
- `apps/web/components/settings/runtime-version-summary.tsx`
- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `apps/web/src/locales/*/agents.json`
- `docs/public/agents-and-profiles.md`
- `docs/specs/agents/requirements/runtime-update-manager.md`
- `docs/specs/agents/system-design/runtime-update-manager.md`
- This work order and its plan.

## Dependencies

Task 02 provides the shared responsive manager and page-owned runtime detail surface.

## Risks

- A compact primary view must keep operation-specific recovery and ownership guidance discoverable.
- Opening a disclosure must not trigger the dialog's Enter-to-confirm behavior.
- The searchable version history and explicit Kandev-default choice must remain distinct.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/runtime-update-manager.md), REQ-AGENTS-RUNTIME-MANAGER-001.
- [System design](../../specs/agents/system-design/runtime-update-manager.md), composition and runtime details.
- [Plan preview](plan.md#ui-03-runtime-detail-and-dirty-policy), UI-03.

## Results

Completed 2026-11-18. Simplified the detail surfaces around one selected target and moved technical context into the collapsed Runtime details disclosure. The four focused Vitest files passed 29 tests. The desktop and phone runtime update Playwright flows each passed one focused test. Settled desktop and phone screenshots were captured from the committed build. Typecheck, i18n checks, and changed-file lint passed during implementation. The linked requirements, design, plan preview, and public guide describe the delivered behavior.

PR fixup verification (2026-11-18): focused Vitest passed 63 tests across five files. The desktop Chromium group passed all 29 scenarios across the full run and a rerun of its 767px boundary case after waiting for drawer animation; the mobile Chromium group passed all 19 scenarios. Fast typecheck, `i18n:check`, changed-file Oxlint, documentation validation, specification lint, and `git diff --check` passed. Fixes cover version-specific success copy, collapsed detail assertions and recovery, touch-sized controls, and review feedback.
