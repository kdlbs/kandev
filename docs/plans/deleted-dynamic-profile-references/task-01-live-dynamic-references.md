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
candidates, delete and disable conflicts report all relevant references, and
both conflict dialogs name the live dynamic profiles. See UI-01 and UI-02 in the
[plan](plan.md).

## In scope

- `ListDynamicProfileReferencesByExecutionProfile` filters
  `deleted_at IS NULL`; `models.DynamicProfileReference` and the controller
  reference type drop the `deleted` flag.
- `DeleteDynamicAgentRoutesForDeletedProfiles` on the dynamic profile
  repository, called best-effort after a profile delete.
- `prepareProfileDeletion` collects all references and returns one
  `ErrProfileInUseDetail`; routing tiers stay a hard blocker.
- An unforced disable collects dynamic profile and utility agent references
  before returning one `ErrProfileInUseDetail`; it leaves the profile enabled.
- Web: `DynamicProfileReference` type, `dynamic_profiles` in the delete server
  action, `profileConflictReferences` for both 409 shapes, the dynamic profile
  sections in both dialogs, and the two new keys in every shipped catalog.

## Out of scope

- Dynamic profile editing, route selection, and routes of live dynamic
  profiles.

## Acceptance

1. A candidate referenced only by a soft-deleted dynamic profile can be deleted
   and disabled without force. Route cleanup runs best-effort; a failed cleanup
   may leave inert routes in the deleted parent.
2. A live dynamic reference blocks without force. Delete reports it with every
   other delete reference in one 409. Disable reports it with utility agent
   references in one 409 and leaves the profile enabled.
3. The delete and disable dialogs list live dynamic profiles and confirm with
   force; saving with a conflict opens the dialog without a generic toast.
4. The disable dialog uses one scrollable phone body and keeps its touch-sized
   Cancel and Disable actions inside a short viewport.

## Verification

Commands ran from the repository root unless the working directory is shown.

- `go test ./internal/agent/settings/controller -count=1` (from
  `apps/backend`): passed.
- `pnpm exec vitest run app/actions/agents.test.ts components/settings/agent-profile-delete-dialog.test.tsx components/settings/agent-profile-page-state.test.ts`
  (from `apps/web`): passed, 32 tests.
- `pnpm run i18n:check` and `pnpm run typecheck` (from `apps/web`): passed.
- `bash scripts/lint-go-changed apps/backend/internal/agent/settings/controller/profile_crud.go apps/backend/internal/agent/settings/controller/profile_crud_dynamic_references_test.go`:
  passed, 0 issues.
- `pnpm run lint` and `pnpm run lint:e2e-sleeps` (from `apps/web`): passed.
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`: passed, 63 tests and 55 published
  pages.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`: passed, 456 decisions and 2045
  specifications validated.
- `pnpm e2e:run --no-build --host --shards 1 --project chromium tests/settings/agent-profile-delete-dynamic.spec.ts`
  (from `apps/web`): passed, 2 tests.
- `pnpm e2e:run --no-build --host --shards 1 --project mobile-chrome tests/settings/mobile-agent-profile-disable-conflict.spec.ts`
  (from `apps/web`): passed, 1 test.

The broader settings test suite and full web test suite were not run.

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
- `apps/web/e2e/tests/settings/agent-profile-delete-dynamic.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-disable-conflict.spec.ts` (new)

## Results

- The new controller regression failed before the implementation because the
  response omitted utility references when a live dynamic reference existed.
  It passed with the combined dependency response, and the stored profile stays
  enabled after the unforced 409.
- The first mobile run showed that a long conflict dialog extended above the
  viewport. After adding a viewport cap and one scroll body, the action height
  check found a 43.575-pixel control. The final mobile run passed with both
  actions at least 48 pixels high and visible in a 320 by 480 viewport.
- The focused desktop and mobile Playwright cases, controller package test,
  32 focused Vitest tests, i18n/type checks, changed-file lint, docs checks,
  and specification checks passed.
