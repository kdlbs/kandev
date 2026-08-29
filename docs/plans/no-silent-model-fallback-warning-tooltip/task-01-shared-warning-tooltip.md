---
id: "01-shared-warning-tooltip"
title: "Compact shared profile warning"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 01: Compact shared profile warning

## Acceptance

1. A profile whose saved start model is absent from the known host catalog
   remains selectable and renders one focusable/hoverable amber warning icon
   without nesting an interactive control in the selected combobox trigger.
2. The existing localized host-probe sentence is absent from the persistent
   option row and appears in a fine-pointer tooltip or coarse-pointer drawer
   when the warning icon is inspected.
3. The create-task desktop and mobile flows retain the warning behavior,
   profile selectability, and mobile no-horizontal-overflow guarantee.

## Verification

From a fresh worktree, first run:

```sh
cd apps && pnpm install --frozen-lockfile
```

Then run the focused checks:

```sh
cd apps/web && pnpm vitest run components/task-create-dialog-options.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run --project chromium tests/settings/no-silent-model-fallback.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts
```

## Files likely touched

- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/combobox.tsx`
- `apps/web/components/task-create-dialog-options.test.tsx`
- `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The unit and E2E assertions share the warning trigger contract.

## Inputs

- `docs/specs/agents/requirements/no-silent-model-fallback.md`, section 8 and frontend test
  requirements.
- `docs/decisions/2026-08-15-executor-authoritative-model-selection.md`, which
  keeps the host probe advisory and the executor authoritative.
- `apps/web/AGENTS.md`, especially tooltip, i18n, and mobile guidance.
- Existing `Combobox` disabled-option tooltip pattern in
  `apps/web/components/combobox.tsx`.

## Risks

- The warning trigger is nested inside a command option. Keep focus and pointer
  handling from accidentally selecting or dismissing the profile option while
  the user inspects the message.
- Radix tooltip content is portalled. E2E selectors must target the visible
  tooltip content, not a hidden or stale portal instance.

## Output contract

Report the summary, actual files changed, exact commands and results, any
mobile-rendered verification, blockers, and synchronized task/plan status.

## Results

Implemented the compact warning trigger in the shared agent-profile option
renderer. The warning remains selectable, has an accessible focusable trigger,
opens in a fine-pointer tooltip or coarse-pointer Drawer, and uses a
noninteractive indicator in the selected combobox trigger. The advisory
sentence is no longer persistent option content.

Verification:

- `cd apps && pnpm install --frozen-lockfile` — passed.
- `cd apps/web && pnpm vitest run components/task-create-dialog-options.test.tsx` — passed, 17 tests.
- `cd apps/web && pnpm exec eslint components/combobox.tsx components/task-create-dialog-options.tsx components/task-create-dialog-options.test.tsx e2e/tests/settings/no-silent-model-fallback.spec.ts e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run i18n:ratchet` — passed, zero new or modified violations.
- `cd apps/web && pnpm e2e:run --project chromium tests/settings/no-silent-model-fallback.spec.ts` — passed, 1 test.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts` — passed, 1 test.
- `git diff --check` — passed.
- Fresh synthetic desktop and mobile screenshots were captured, inspected, and
  compressed for the PR description.
- Review remediation completed: the selected trigger no longer nests an
  interactive warning button, and coarse-pointer inspection uses the shared
  Drawer pattern.
