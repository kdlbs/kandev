---
id: "04-compatibility-acceptance"
title: "Prove user flows and update public guidance"
status: done
wave: 3
depends_on: ['03-profile-strictness-controls']
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.2
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.4
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.7
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.8
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.9
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.10
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
---
# Task 04: Prove user flows and update public guidance

## Summary

Complete desktop/mobile acceptance and align public guidance with the implementation.

## In scope

Complete desktop/mobile acceptance and align public guidance with the implementation.

Extend all four existing specs named in the plan's E2E matrix. Keep strict
fixtures explicit and add omitted-field compatible fixtures. Edit/save/reload
through the UI; inspect visible errors and one persisted warning. Retain backend
pre-prompt assertions from Task 02; chat visibility alone does not prove no inference.
Cover both editor surfaces, dormant fallback restoration, strict+empty validation,
keyboard interactions, drawer dismissal/focus, touch bounds, and overflow.

Update the reference sections in `docs/public/agents-and-profiles.md` and the
explanation in `docs/public/executors.md`. Explain per-profile default off, upgrade
compatibility, exact full-ID selection, and ordinary apply errors vs automatic
fallback. Search README/screenshots and linked specs/decisions for old implicit
strict claims. Keep completed transport fixes and managed-runtime documentation.

Reconcile historical Task 01's scope notice, all new work-order results, and
specification status after actual implementation checks pass. Refresh PR head/base
read-only before handoff. Do not push, comment, or commit under this work order.

## Out of scope

Global policy, provider authentication, Office post-start routing redesign,
unrelated cleanup, and publication.

## Acceptance

- Desktop and phone users can save strictness and observe the correct launch
  result; compatible warnings survive reload exactly once. Touch and keyboard
  interactions meet the design and both editor paths have rendered coverage.
- Public profile/executor guidance matches the new default, fallback precedence,
  and error matrix, including legacy upgrade behavior.
- Every required task check has a recorded result; pending or skipped Postgres
  evidence remains explicit. Completed transport/reuse regressions still pass.

## ASCII UI preview

Apply [UI-01 and UI-02](plan.md#ascii-ui-preview). This work order verifies those
views and updates guide text; it does not introduce another layout.

```text
Desktop: Strict [off/on] above [Automatic] [Explicit] cards -> Save
Phone:   Strict [off/on] -> Automatic -> Explicit -> Save
Strict on: both fallback cards disabled, saved values visible.
Help: tap -> inset drawer -> dismiss -> focus returns -> save draft.
```

These views map to AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6 and .7.

## Verification

Run from the repository root. Use TDD for changed logic; first record the
behavioral failure, then run the listed checks after implementation.

```bash
(cd apps/web && pnpm e2e:run tests/settings/no-silent-model-fallback.spec.ts tests/session/model-mismatch-warning.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts tests/session/mobile-model-mismatch-warning.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/mobile-model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning-helpers.ts`
- `docs/public/agents-and-profiles.md`, `docs/public/executors.md`
- This plan, work orders, and paired requirement/design status and results.

## Dependencies

['03-profile-strictness-controls']; preserve Task 01 transport and candidate-lookup fixes.

## Risks

Use fresh managed production builds: do not pass --no-build after code changes.
Run E2E commands sequentially with the guarded worker budget. Restore shared
fixture mutations. Mock browser results cannot certify external provider auth.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/no-silent-model-fallback.md)
- [Policy design](../../specs/agents/system-design/no-silent-model-fallback-01.md)
- [Recovery/UI design](../../specs/agents/system-design/no-silent-model-fallback-02.md)
- [Plan and regression matrix](plan.md)
- Scoped AGENTS.md, existing tests beside the affected code, and compatibility
  behavior at `ba960f973205854733e0dcd9afa355bdda3dddf6`.

## Results

Implemented on 2026-09-15. Public agent/profile and executor guidance now
describes compatible-by-default behavior, explicit strict opt-in, fallback
precedence, host-probe advisories, and upgrade compatibility. The existing
desktop and mobile settings and session mismatch specs passed with fresh
production builds:

- desktop settings: 3 passed;
- mobile settings: 3 passed;
- desktop session mismatch: 2 passed;
- mobile session mismatch: 2 passed.

Public-doc and specification validators passed. The Postgres check remains
pending until an isolated test database is available.
