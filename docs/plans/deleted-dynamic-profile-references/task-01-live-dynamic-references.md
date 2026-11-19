---
id: "01-live-dynamic-references"
title: "Count only live dynamic references and show them"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.32
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Count only live dynamic references and show them

## Summary

Soft-deleted dynamic profiles stop blocking delete and disable of their
candidates, the delete 409 reports every blocker at once, and both conflict
dialogs name the live dynamic profiles. See UI-01 and UI-02 in the
[plan](plan.md).

## In scope

- `ListDynamicProfileReferencesByExecutionProfile` filters
  `deleted_at IS NULL`; `models.DynamicProfileReference` and the controller
  reference type drop the `deleted` flag.
- `DeleteDynamicAgentRoutesForDeletedProfiles` on the dynamic profile
  repository, called best-effort after a profile delete.
- `prepareProfileDeletion` collects all references and returns one
  `ErrProfileInUseDetail`; routing tiers stay a hard blocker.
- Web: `DynamicProfileReference` type, `dynamic_profiles` in the delete server
  action, `profileConflictReferences` for both 409 shapes, the dynamic profile
  sections in both dialogs, and the two new keys in every shipped catalog.

## Out of scope

- The disable path's blocker order and the routes of live dynamic profiles.

## Acceptance

1. A candidate referenced only by a soft-deleted dynamic profile can be deleted
   and disabled without force, and its routes in that parent are removed.
2. A live dynamic reference blocks without force, is reported together with
   active sessions in one 409, and force still deletes or disables.
3. The delete and disable dialogs list live dynamic profiles and confirm with
   force; saving with a conflict opens the dialog without the generic toast.

## Verification

```bash
# from the repository root
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/agent/settings/...)
(cd apps/web && pnpm exec vitest run app/actions/agents.test.ts \
  components/settings/agent-profile-delete-dialog.test.tsx \
  components/settings/agent-profile-page-state.test.ts)
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
(cd apps/web && pnpm e2e:run tests/settings/agent-profile-delete-dynamic.spec.ts)
```

## Likely files

- `apps/backend/internal/agent/settings/store/{sqlite,store}.go`
- `apps/backend/internal/agent/settings/controller/{controller,profile_crud}.go`
- `apps/backend/internal/agent/settings/controller/profile_crud_dynamic_references_test.go` (new)
- `apps/backend/internal/agent/settings/models/dynamic.go`
- `apps/web/app/actions/agents{.ts,.test.ts}`
- `apps/web/components/settings/agent-profile-delete-dialog{.tsx,.test.tsx}`
- `apps/web/components/settings/agent-profile-page-state{.ts,.test.ts}`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/lib/types/agent-profile-errors.ts`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/agent-profile-delete-dynamic.spec.ts` (new)

## Results

- Before the change, `TestDeleteProfile_IgnoresSoftDeletedDynamicReferences`,
  `TestUpdateProfile_DisableIgnoresSoftDeletedDynamicReferences`, and
  `TestDeleteProfile_ReportsDynamicAndSessionReferencesTogether` failed, and
  the Vitest cases for the `dynamic_profiles` conflict, both dialogs, and the
  save toast failed; all pass with the change.
- `pnpm run i18n:check`, `pnpm run typecheck`, the i18n and E2E sleep
  ratchets, Oxlint, and Prettier pass on the changed files.
- On the Windows development host, `internal/agent/settings/controller` and
  `internal/agent/settings/store` show the same failures before and after the
  change (Copilot plugin discovery, managed npm, and timestamp-ordering tests
  such as `TestAgentGETsUseProfileOrderSnapshot`, which fails intermittently on
  both); none involves profile references.
- Playwright on the `chromium` project: both cases of
  `e2e/tests/settings/agent-profile-delete-dynamic.spec.ts` pass (they enable
  `features.dynamicAgentRouting` through the backend fixture), and the
  existing `agent-profile-delete.spec.ts` and `agent-profile-disable.spec.ts`
  still pass (10 tests). The spec captures the conflict dialog for the PR when
  `CAPTURE_PR_ASSETS` is set.
