---
id: "01-project-and-render-approval-badge"
title: "Project and render the workflow approval badge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.2
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.3
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.5
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 01: Project and render the workflow approval badge

## Summary

Deliver the approval badge through the existing bounded task feed and hydrated PR records.
Use TDD for eligibility, projection convergence, badge priority, and accessible explanations.

## In scope

- Extend the bounded input, observation, summary, rebuild adapter, and live-event decoding described in the design.
- Add the compact TypeScript boolean and map it to `TaskPRInfo`.
- Pass an optional approval prop through `PRTaskIconGlyph` to `PRStatusGlyph`.
- Use the filled amber lock, conflict priority, current automation positions, and existing localization keys.
- Preserve the approval reason during compact disclosure hydration failure; use full evidence after hydration.
- Add the unit/component regressions named in the plan, including multi-PR and legacy payloads.

## Out of scope

- Provider collection/classification changes, storage migrations, topbar badges, and GitHub approval actions.
- Browser scenario authoring and public documentation, owned by Task 02.

## Acceptance

1. Adapter, live-event, rebuild, and frontend mapping tests prove the same explicit approval flag with current-head and lifecycle guards.
2. Component tests prove compact/full eligibility, conflict priority, automation coexistence, localized accessible names, and authoritative hydration clearing.
3. The change adds no provider request, session subscription, runtime flag, or database column; existing observation/readiness regressions pass.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [combined previews](plan.md#ascii-ui-preview):

```text
[done] Review Contributor PR #4082 [PR+lock]
CI     Awaiting maintainer approval

Conflicts plus approval: [PR+!]    (red triangle)
CI     Awaiting maintainer approval
Merge  Conflicts

Compact disclosure:
Awaiting maintainer approval
Loading PR details...             (or unavailable)
```

UI-03 uses the same glyph and text in the existing phone drawer.
Apply 003.1-003.8; keep the current touch target, scroll owner, and task-navigation behavior.
Task 02 owns rendered phone and desktop proof.

## Verification

Run from the repository root. Install dependencies once when this worktree lacks a completed install.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/task/statussummary -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'StatusSummary' -count=1)
(cd apps/web && pnpm exec vitest run lib/task-pr-info.test.ts components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon-conflicts.test.ts components/github/pr-task-icon.automation.test.ts components/github/pr-workflow-attention.test.ts components/github/pr-workflow-attention-icon.test.ts)
(cd apps/web && pnpm exec vitest run components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-status-summary.test.ts components/github/pr-workflow-attention-summary.test.ts --reporter=dot)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run any additional changed suite by exact path and record it here.
Add meaningful failing tests before production edits. Missing selectors alone do not prove behavior RED.
If a new locale key becomes necessary, supply all six languages and generate Traditional Chinese with `pnpm run i18n:zh-hant`.

## Files likely touched

- `apps/backend/internal/backendapp/status_summary_adapter.go` and `status_summary_adapter_test.go`
- `apps/backend/internal/task/statussummary/{model.go,rebuild.go,projector.go,projector_events.go,projector_pr.go}`
- New `apps/backend/internal/task/statussummary/projector_workflow_approval_test.go`
- `apps/web/lib/types/task-status-summary.ts`, `apps/web/lib/task-pr-info.ts`, and new `apps/web/lib/task-pr-info.test.ts`
- `apps/web/components/github/{pr-task-icon.tsx,pr-task-icon-disclosure.tsx,pr-task-status-summary.tsx,pr-status-glyph.tsx}`
- `apps/web/components/github/pr-task-icon.render.test.tsx`, new `pr-task-icon.workflow-approval.test.tsx`, and related changed test suites

## Dependencies

None. Existing stored workflow attention and shared task projection are available.

## Risks

Handle malformed event evidence without treating missing values as approval.
The compact flag aggregates across open PRs; the representative identity alone cannot define it.
Full negative evidence must replace the compact positive without retaining a false badge.

## Parallelism

`sequential`

## Inputs

- [Requirement 003](../../specs/integrations/requirements/github-workflow-attention.md#req-integrations-github-workflow-attention-003-task-approval-badge)
- [Badge design](../../specs/integrations/system-design/github-workflow-attention.md#task-approval-badge)
- Existing conflict and automation projection/component tests
- Scoped backend and web `AGENTS.md`, `/tdd`, and `/mobile-parity`

## Results

Implemented the bounded backend projection and task icon. Approval requires an open PR and a matching current head; the compact flag aggregates across open PRs. Conflict warnings keep priority, and hydrated PRs override the compact fallback.

- `go test ./internal/task/statussummary -count=1`: passed.
- `go test ./internal/backendapp -run 'StatusSummary' -count=1`: passed.
- `pnpm exec vitest run components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-status-summary.test.ts components/github/pr-workflow-attention-summary.test.ts --reporter=dot`: 49 tests passed. This includes same-head stale approval feedback by PR, lock retention, and current-head negative or changed-head clearing.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: passed.
- `pnpm run i18n:ratchet`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
