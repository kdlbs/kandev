---
id: "01-draft-gating-mark-ready"
title: "GitLab draft gating and Mark as ready"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.31
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.32
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.33
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.34
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.35
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-03.md
---

# Task 01: Draft gating and Mark as ready

## Summary

Disable merge controls for draft MRs with an explicit hint, map GitLab's merge
refusals to fixed Kandev messages (no provider text), and add a separate
explicit "Mark as ready" action.

## In scope

- Backend: pure `stripDraftMarkers` helper; `PATClient.MarkMRReady`; add to the
  `Client` interface plus mock and noop; `PUT /mrs/ready` route through
  `RunWithWorkspaceClient`; `ErrMRIsDraft` guard in `MergeMR` (PAT and mock); `MergeRefusedError` and `writeMergeActionError` (merge route only); `writeMarkReadyError`; `ErrMRNilResult`; mock feedback-read counter; server-side cached task-MR refresh after the ready write (`syncTaskMRWithClient` for matching workspace rows, then `publishTaskMRUpdated`; sync-mode comparison reconcile, never `AssociateExistingMRByURL`).
- Web: `useMRFeedback.reload` (ok/failed/superseded) and the bounded 5-reload post-ready wait; shared draft predicate; gate the detail-panel and popover merge
  controls; Mark as ready button and `markMRReady` API fn; refresh after
  action; `gitlab:` i18n keys in seven locales.
- Tests as listed in the plan, written first (TDD).

## Out of scope

GitHub, source-branch removal, auto-clearing drafts in merge/automation.

## Acceptance

- Each AC .31-.35 has a passing test; merge write is never issued for a draft.
- Targeted Go and Vitest suites, changed-Go lint, web lint/typecheck,
  `pnpm run i18n:check`, `pnpm run i18n:ratchet`, the Playwright spec, spec lint
  and docs validation pass.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/gitlab)
bash scripts/lint-go-changed <changed-go-files>
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm vitest run components/gitlab hooks/domains/gitlab)
(cd apps/web && pnpm e2e:run e2e/tests/gitlab/mr-draft-merge.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/gitlab/{client.go,pat_client_actions.go,mock_client.go,noop_client.go,controller_watches.go,service_merge.go}`
- `apps/web/components/gitlab/{mr-detail-panel.tsx,mr-merge-button.tsx,mr-ci-popover.tsx}`, `apps/web/components/**/mr-task-icon.tsx`
- `apps/web/hooks/domains/gitlab/{use-mr-actions.ts,use-mr-feedback.ts}`, `apps/web/lib/types/gitlab.ts`, `apps/web/lib/api/domains/gitlab-api.ts`
- `apps/web/e2e/tests/gitlab/mr-draft-merge.spec.ts`
- `apps/web/src/locales/*/gitlab.json`
