---
id: "05-ownership-ui"
title: "Complete ownership UI and documentation"
status: done
wave: 5
depends_on:
  - "04-autonomous-attribution"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-002
  - REQ-EXECUTORS-OWNERSHIP-003
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-EXECUTORS-OWNERSHIP-005
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-001.1
  - AC-EXECUTORS-OWNERSHIP-001.2
  - AC-EXECUTORS-OWNERSHIP-001.3
  - AC-EXECUTORS-OWNERSHIP-001.4
  - AC-EXECUTORS-OWNERSHIP-002.1
  - AC-EXECUTORS-OWNERSHIP-002.2
  - AC-EXECUTORS-OWNERSHIP-002.3
  - AC-EXECUTORS-OWNERSHIP-002.4
  - AC-EXECUTORS-OWNERSHIP-003.1
  - AC-EXECUTORS-OWNERSHIP-003.2
  - AC-EXECUTORS-OWNERSHIP-003.3
  - AC-EXECUTORS-OWNERSHIP-003.4
  - AC-EXECUTORS-OWNERSHIP-004.1
  - AC-EXECUTORS-OWNERSHIP-005.1
  - AC-EXECUTORS-OWNERSHIP-005.2
  - AC-EXECUTORS-OWNERSHIP-005.3
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 05: Complete ownership UI and documentation

## Summary

Expose ownership controls and eligible executor choices on desktop and phone, and document the final user-facing boundary.

## In scope

- Extend executor API/types/store and complete profile editor/hub with owner status, self/shared choices, and manager assignment.
- Use the filtered catalogue everywhere, reject revoked defaults without silently choosing another machine, and preserve drafts through denial/refetch.
- Add seven-language copy and desktop/mobile authenticated E2E; update authentication, team, executor and automation public guidance and the existing auth shared-surface qualification.

## Out of scope

New grants, new provider runtimes, and ownership of unrelated shared configuration.

## Acceptance

- Settings create/edit/clear/reassign/discard/reload work for authorized users with provider restrictions preserved.
- A/B/admin desktop and phone tests prove filtered choices, denied API calls, live eviction, invalid-default recovery, and shared compatibility.
- Phone interaction matches UI-02 with measured touch/focus/scroll containment; public docs and locale checks agree with implemented behavior.

## ASCII UI preview

### UI-01: Settings ownership (desktop)

Entry: Settings > Executors > a visible profile. State: configuration manager.

```text
Profile: Personal SSH
Ownership     [ Ana Silva                 v ]
              Shared | Me | Choose user...
[ Discard ]                         [ Save changes ]
```

The owner sees Shared and Me; another user's assignment is manager-only.
Shared is the explicit unowned value. Ownership-only changes do not unlock
provider configuration controls. Save failure retains the draft and exposes
an accessible error; loading user choices is distinct from no choices.

### UI-02: Settings ownership (phone)

Entry: the same direct profile route. State: choosing an owner.

```text
< Executors
Personal SSH
Ownership
[ Ana Silva                              v ]
... existing profile sections ...
[ Save changes ]

+------------------------------------------+
| Ownership                          Close |
| [ Search users...                      ] |
| Shared                                   |
| Me                                       |
| Ana Silva                            (o) |
+------------------------------------------+
```

Use direct navigation for the full editor and an inset bottom Drawer for the
temporary choice. The form owns page scrolling; an overflowing drawer owns its
option scrolling with a fixed title. Preserve safe-area clearance, keyboard
focus return, at least 44px phone/coarse-pointer targets, and no page overflow.
Desktop ordinary controls retain 28px density. Names and spacing are illustrative;
control order, permitted options, semantic labels, and responsive surfaces are
structural requirements. Reuse the existing Settings save/discard behavior.

### UI-03: Task-create revoked default (both viewports)

```text
Executor       [ Choose an available executor   v ]
This executor is no longer available. Choose another.
[ Cancel ]                            [ Start: disabled ]
```

Eligible shared/owned options are the only returned choices. Do not display the
revoked profile's name/configuration or silently substitute another machine.
Retain the task draft while the user repairs selection. On phone, use the
existing task-create picker surface with the same filtered state. All preview
copy is illustrative and must use translated locale keys in implementation.

These views map to AC-EXECUTORS-OWNERSHIP-002.4 and
AC-EXECUTORS-OWNERSHIP-003.1 through .4. Rendered proof belongs to Task 05.

[Full combined preview](plan.md#ascii-ui-preview).

## Verification

Run from repository root. On a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` before the first pnpm command.
Add the planned tests with TDD; test names must contain `ExecutorOwnership`
where the Go commands filter by that pattern. Ensure new focused web tests are
added to the exact Vitest command before recording completion.

```bash
(cd apps/web && pnpm exec vitest run \
  lib/auth/executor-ownership.test.ts lib/ws/handlers/executor-ownership.test.ts \
  components/task-create-dialog-defaults.test.ts components/task-create-dialog-state-seeded-executor.test.ts \
  components/task-create-dialog-effects-executor.test.ts components/task-create-dialog-footer.test.ts \
  components/task-create-dialog-prop-builders.test.ts \
  components/settings/profile-edit/use-executor-profile-save-contributor.test.tsx \
  'app/settings/executors/new/[type]/create-profile-catalogue-publication.test.tsx' \
  'app/settings/executors/new/[type]/remote-docker-create-page.test.tsx' \
  components/settings/plugin-executor-profile.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run --project auth tests/auth/executor-ownership.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-executor-ownership.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/types/executor.ts`
- `apps/web/lib/types/executor-payloads.ts`
- `apps/web/app/actions/executors.ts`
- `apps/web/lib/api/`
- `apps/web/hooks/domains/settings/`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-defaults.ts`
- `apps/web/app/settings/executors/`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/auth/executor-ownership.spec.ts (new)`
- `apps/web/e2e/tests/auth/mobile-executor-ownership.spec.ts (new)`
- `docs/public/authentication.md`
- `docs/public/team-access.md`
- `docs/public/executors.md`
- `docs/public/automation-and-mcp.md`
- `docs/specs/auth/requirements/auth.md`
- `docs/specs/auth/requirements/roles-and-scopes.md`

## Dependencies

Task 04; execute sequentially after its acceptance and targeted checks pass.

## Risks

Catalogue state and store reads must not reinsert revoked profiles. Auth E2E must isolate and reset backend auth, including the mobile project.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/per-user-ownership.md).
- [System design](../../specs/executors/system-design/per-user-ownership.md).
- [Ownership ADR](../../decisions/2026-10-15-private-executor-admission.md).
- [Plan evidence and existing code patterns](plan.md#technical-approach).
- Read scoped AGENTS.md, `/tdd`, and applicable `/mobile-parity` and `/e2e`
  instructions before implementation.

## Results

Implemented ownership controls in built-in and plugin profile editors and creation flows, plus non-system executor ownership. Administrators choose active directory users; members choose themselves on creation or clear their own ownership. Parent restrictions remain visible. Phone/coarse-pointer choices use an inset drawer, and fine-pointer phone widths use that same composition. Draft reconciliation preserves in-flight edits; revoked task selections remain selected and block submission until repaired.

Added English, Portuguese, Simplified/Traditional Chinese, Japanese, Korean, and pseudo-locale copy. Updated public executor, authentication, team-access, automation/MCP, and WebSocket guides plus README. The 104 focused web tests, full web lint, translations, and public-doc checks pass. All five desktop and four phone scenarios pass, including actual owner launch, collaborator denial, live eviction, and draft preservation across the phone breakpoint with a fine pointer. Final typecheck, web lint, and the production web build pass. The rendered phone drawer was inspected against the plan preview; capture: `apps/web/.pr-assets/executor-ownership-mobile.png`.

PR remediation: ownership controls reuse the shared directory domain hook. Reproduced and fixed unsaved draft loss during an unrelated catalogue event; all six desktop and five phone scenarios pass. The four stale task-create test fixtures now include their selected eligible profile; 34 focused frontend tests, typecheck, full lint, and Vite build pass.
