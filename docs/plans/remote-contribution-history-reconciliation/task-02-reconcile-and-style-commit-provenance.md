---
id: "02-reconcile-and-style-commit-provenance"
title: "Reconcile and style commit provenance"
status: complete
wave: 2
depends_on: ["01-recover-provider-commit-evidence"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 02: Reconcile and Style Commit Provenance

Use complete provider evidence without requiring an upstream. Sort remote commits correctly. Remove
the provider-error banner and show proven commit provenance with accessible colors.

## Inputs

- Spec sections: What, Failure modes, and Scenarios.
- Plan sections: Evidence classification, Commit reconciliation, and Desktop and mobile contract.
- Decision: `docs/decisions/2026-08-13-provider-history-changes-enrichment.md`.
- Task 01 provider evidence and refresh contract.

## Acceptance

1. After equal heads are classified as aligned, complete provider history with local HEAD and a
   different/newer provider head classifies as `provider_ahead` without an upstream.
2. Provider-ahead without an upstream disables Push and does not enable Pull.
3. Unknown evidence does not expose replacement or provider-adoption actions.
4. Provider-only commits appear newest first within each repository and use `current_pr` provenance.
5. Shared commits use the neutral marker. Ordinary unpushed commits keep the emerald arrow.
6. Confirmed divergence uses violet current-PR markers and amber local-checkout markers.
7. Marker title, screen-reader text, and a stable data attribute expose provenance without color.
8. The Changes panel never renders the provider-history warning for a failed provider read.
9. Disabled action tooltips describe the actual relation and do not reuse the removed warning as a
   generic reason.

## Files Likely Touched

- `apps/web/hooks/domains/session/remote-contribution-relation.ts`
- `apps/web/hooks/domains/session/remote-contribution-relation.test.ts`
- `apps/web/components/task/changes-panel-helpers.ts`
- `apps/web/components/task/changes-panel-remote.test.ts`
- `apps/web/components/task/commit-row.tsx`
- `apps/web/components/task/commit-row.test.tsx`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-body.tsx`
- `apps/web/components/task/changes-panel-timeline.tsx`
- `apps/web/components/task/changes-panel-header.tsx`
- `apps/web/components/task/changes-panel-per-repo-menu.tsx`
- `apps/web/components/vcs-split-button-dropdown.tsx`
- `apps/web/components/vcs-multi-repo-menu.tsx`
- `apps/web/components/task/mobile/mobile-git-actions-dropdown.tsx`

## TDD Sequence

1. Add failing relation tests for provider-ahead without upstream and fail-closed actions.
2. Add failing merge tests for provider-only order and provenance.
3. Add failing row tests for visible colors, accessible labels, and the data attribute.
4. Implement classification and merge changes.
5. Remove warning plumbing and correct disabled action titles.
6. Refactor shared presentation code after focused tests pass.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/session/remote-contribution-relation.test.ts components/task/changes-panel-remote.test.ts components/task/commit-row.test.tsx
```

## Dependencies

Task 01.

## Parallelism

Sequential. Relation policy, merged commit data, and row styling form one visible contract.

## Risks

- Do not enable Pull from provider evidence alone when the checkout has no upstream.
- Do not mark shared commits as provider-only.
- Do not remove the compact control for a confirmed divergence.
- Use existing theme-safe violet and amber tokens. Do not hardcode a color without dark-theme proof.

## Output Contract

Report the relation rules, commit order, provenance mapping, removed warning path, files changed, and
exact test results. Update this task and `plan.md` in the same conversation.

## Results

- After equal heads are classified as aligned, complete provider history containing local HEAD and a
  different provider head now proves `provider_ahead` without an upstream.
  Push remains disabled; Pull requires a configured upstream. Unknown and diverged states keep their
  existing fail-closed action policy with relation-specific reasons.
- Provider-only commits are reconciled by SHA, rendered newest first within each repository with
  `current_pr` provenance, and shared commits remain neutral. Confirmed divergence uses violet
  current-PR and amber local-checkout markers, each with title, screen-reader text, and stable data
  attributes.
- Removed provider-history warning and empty/error body plumbing while retaining internal provider error
  evidence for safety classification. Desktop and mobile action tooltips use the specific relation
  reason keys.
- Task-focused command passed 49 tests across the three task files. The final six-file focused command
  passed 59 tests.
- PR fixup now disables remote Push and Pull while provider evidence is unavailable, orders named
  repositories independently, and passes the repository scope through single-repository resolution
  actions.
