---
id: "01-render-model-only-title"
title: "Render model-only agent tab titles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-model-configuration-summary.md"
---

# Task 01: Render Model-only Agent Tab Titles

## Acceptance

- An unnamed agent tab resolves to the authoritative current model display name without appending
  session mode or non-model ACP config values.
- A live model switch and task-detail reload retain the model-only title.
- A user-supplied session name still overrides every derived title, and task-chat/profile model
  selector summaries remain unchanged.

## Verification

```bash
cd apps && rtk pnpm --filter @kandev/web test -- --run components/task/session-tab-title.test.ts
cd apps/web && rtk pnpm e2e:run tests/chat/model-selector-error.spec.ts -- --grep "agent tab keeps the selected model title after reload"
```

## Files Likely Touched

- `apps/web/components/task/session-tab-title.ts`
- `apps/web/components/task/session-tab-title.test.ts`
- `apps/web/e2e/tests/chat/model-selector-error.spec.ts`
- `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts` (PR evidence capture only)
- `docs/specs/ui/requirements/acp-model-configuration-summary.md`
- `docs/plans/agent-tab-model-only/plan.md`
- `docs/plans/agent-tab-model-only/task-01-render-model-only-title.md`

## Dependencies

None.

## Parallelism

`sequential`. The resolver, its unit contract, and the existing persistence E2E form one focused
behavioral change.

## Inputs

- Spec **What** and unnamed-agent-tab **Scenario**.
- PR #2021 and its `resolveSessionTabTitle` precedence and reload regression.
- Existing `SessionPage.sessionTabBySessionId` stable E2E locator.
- `TaskLayout` mobile/tablet branching, which confirms this Dockview tab has no mobile surface.

## TDD Sequence

1. Change the non-model-config unit case to require the exact model-only label and run the focused
   Vitest command to confirm RED.
2. Remove non-model config composition from the derived model title without changing title
   precedence or selector code, then rerun Vitest to GREEN.
3. Tighten the existing model-switch persistence E2E to require the exact tab label before and
   after reload, then run the managed focused E2E against freshly built assets.
4. Mark this task `done`, mark the plan checkbox complete, and return the spec to `shipped`.

## Risks

- Model display names can come from either the model list or the live model config option; removing
  the extra values must not bypass the provider-supplied model label.
- Dockview restoration can reapply stale panel titles; the existing session-tab component sync and
  reload E2E must continue proving the rendered tab wins.

## Output Contract

Report the expected RED assertion, resolver change, exact unit and E2E command results, files
changed, mobile no-surface confirmation, remaining risks, and task/plan/spec status updates.
