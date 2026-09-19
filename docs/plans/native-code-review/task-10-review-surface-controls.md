---
id: "10-review-surface-controls"
title: "Review changes control, run status, findings overview, step editor"
status: pending
wave: 7
depends_on: ["09-diff-finding-annotations"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 10: Review surface controls

The trigger, the run status, the findings overview, and the workflow step setting.

## Inputs

- Spec **What**, the `review_agent_unavailable` scenario, and the run **State machine** for the states the control must show.
- `components/review/review-top-bar.tsx` — where the control goes; `ReviewWalkthroughButton` shows the disabled-tooltip pattern (`apps/web/AGENTS.md` requires the focusable-span wrapper on a disabled `Button`).
- `components/review/review-comments-overview.tsx` — the overview layout to mirror.
- `components/task/{changes-panel-header.tsx,changes-top-bar.tsx}` — the Changes-panel toolbar.
- `apps/web/AGENTS.md` → self-documenting settings and settings-save-coordination rules for the step editor.

## Work

1. `components/review/review-run-button.tsx` — **Review changes** action with states: idle; running (spinner + Cancel); completed (open-finding count badge); failed (error affordance). On `review_agent_unavailable` it renders an inline message naming **Settings → Utility Agents** with a link, not a bare toast. On `review_no_changes` it shows a muted "No changes to review".
2. `components/review/review-top-bar.tsx` — mount the control and the findings count; keep the file under 600 lines by extracting rather than inlining.
3. `components/review/review-findings-overview.tsx` — findings grouped by repository then file, severity-sorted, each row selecting + scrolling to the anchored finding; separate sections for open, resolved/dismissed, and "not in current changes".
4. `components/review/{review-diff-list.tsx,review-file-tree.tsx}` — per-file open-finding badge beside the existing comment badge, keyed by `reviewFileKey`.
5. `components/task/{changes-panel-header.tsx,changes-top-bar.tsx}` — the same control on the Changes panel, sharing `review-run-button.tsx`.
6. Workflow step editor (under `components/settings/workflows/`) — a **Run code review on entry** control plus an optional agent-profile picker, with visible copy stating that the review runs against the task's changed files, that the chosen profile's model does the reviewing, that findings are advisory, and that a failed review does not block the step. Register through `useSettingsSaveContributor` / `SettingsPageTemplate`; no page-local save button.

## Acceptance

- The control reflects every run state and offers Cancel only while running.
- With no capable agent, the inline Settings message appears and no run is created.
- The overview groups per repository and navigating a row scrolls to that finding.
- The step editor persists the action through the shared save coordinator and round-trips on reload.

## Verification

```
cd apps/web && pnpm vitest run components/review components/task/changes-panel-header.test.tsx components/settings/workflows
cd apps/web && pnpm run typecheck && pnpm lint
```

## Files likely touched

`components/review/{review-run-button.tsx,review-findings-overview.tsx,review-top-bar.tsx,review-diff-list.tsx,review-file-tree.tsx}`, `components/task/{changes-panel-header.tsx,changes-top-bar.tsx}`, `components/settings/workflows/*`, plus tests.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
