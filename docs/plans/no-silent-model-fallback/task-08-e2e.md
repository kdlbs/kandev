---
id: task-08-e2e
title: E2E: gone start model flows on the mock backend
status: done
wave: 4
depends_on: [task-05-picker-gone-support, task-06-profile-editor, task-07-profile-picker-gating]
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 8 — Playwright E2E for gone-model flows

## Change

Add an E2E spec (e.g. `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`)
running on the mock backend (`KANDEV_E2E_MOCK=true` fixture). The mock agent
advertises a fixed model list; create a profile whose start model is NOT in
that list (via the settings API or UI) to establish a "gone" model.

Assert:

1. **Profile editor**: the gone start model renders red + disabled in the
   model picker (option greyed, reason tooltip); the current value shows
   the gone model id.
2. **Fallback row + toggle**: with `auto_fallback` off, the "Agent
   fallback" row is visible; enabling the "Fallback automatically to next
   model" toggle hides it; the toggle persists after save.
3. **Task-create profile picker**: the strict-gone profile is greyed and
   unselectable (reason tooltip); after setting a fallback model, the
   profile becomes selectable and shows the fallback warning.
4. Saving the profile keeps the gone start model (no auto-heal on save).

Check `apps/web/e2e/README.md` for fixture conventions (backend setup,
selectors, mock config) and reuse existing settings/office specs for the
patterns (`utility-agents.spec.ts`, `office-routing-blocked.spec.ts`).

## Acceptance

1. The new spec passes on the mock backend.
2. Existing e2e suite still passes (no regressions in profile/model flows).

## Verification

```sh
cd apps/web && pnpm e2e --project=chromium tests/settings/no-silent-model-fallback.spec.ts
# then the full suite
cd apps/web && pnpm e2e
```

## Risks

- The mock agent's model list is fixed in `apps/backend/internal/agent/agents/mock.go`
  — verify the advertised IDs so the test's "gone" model is genuinely
  absent (and not silently present via a config-option path).
- The task-create picker requires the capabilities probe to have landed;
  use the existing `agentProfilesLoading` / capability gates.
