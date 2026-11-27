---
id: "05-authoring"
title: "Authoring and adoption guidance"
status: done
wave: 5
depends_on:
  - "04-compatibility"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
acceptance_criteria:
  - AC-PLUGINS-ACTION-UX-001.1
  - AC-PLUGINS-ACTION-UX-001.2
  - AC-PLUGINS-ACTION-UX-001.4
  - AC-PLUGINS-ACTION-UX-002.4
  - AC-PLUGINS-ACTION-UX-003.1
  - AC-PLUGINS-ACTION-UX-003.2
  - AC-PLUGINS-ACTION-UX-003.3
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
---

# Task 05: Authoring and adoption guidance

## Summary

Publish the implemented author contract in repository documentation.
Provide a safe migration recipe and the per-plugin follow-up sequence.

## In scope

- Update the Action/ActionGroup signatures and location matrix in the public authoring reference and PLUGIN-API.
- Add examples for a command, toggle/recording state, text value, bounded animated icon, controlled overlay, and old-host fallback.
- Explain that raw component slots and host.ui.Button remain supported.
- Update web AGENTS guidance and the adoption map with actual host verification results.
- Document all composer locations and the compact tablet status exception.
- Keep requirement/design and plan statuses accurate after all work-order checks pass.

## Out of scope

Editing external plugin repositories, inventing a supporting release number, committing, pushing, or publishing packages.

## Acceptance

1. SDK, implementation, authoring reference, and examples agree on props, supported locations, and fallback behavior.
2. Documentation clearly separates backward compatibility from optional visual adoption and names each official plugin's migration needs.
3. Package results include exact checks, limitations, and a complete traceability mapping without claiming untested release compatibility.

## Verification

Run from the repository root. Use TDD for new behavior and record the initial
behavioral failure. New test paths in this order are files to create.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/packages/plugin-sdk && pnpm test && pnpm typecheck)
(cd apps/web && pnpm exec vitest run lib/plugins/sdk-contract.test.ts lib/plugins/action-compatibility.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint components/actions components/plugins/plugin-action.tsx components/plugins/plugin-action-surface.tsx components/plugins/plugin-slot.tsx components/kanban/main-top-bar-plugin-actions.tsx components/kanban/kanban-header.tsx components/task/task-top-bar-plugin-actions.tsx components/task/task-top-bar.tsx components/task/chat/chat-input-plugin-actions.tsx components/task/chat/chat-input-toolbar-primitives.tsx components/task-create-dialog-selectors.tsx components/app-sidebar/app-sidebar-workspace-actions.tsx components/app-sidebar/app-sidebar-new-task-item.tsx components/app-status-bar/app-status-bar-plugin-slots.tsx components/app-status-bar/lsp-status-item.tsx lib/plugins/host-api.ts lib/plugins/types.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run any other test file changed by this work order with the same targeted runner.
For rendered changes, inspect the focused desktop and phone screenshots against
the assigned previews. Do not infer CSS geometry from unit tests.

## Files likely touched

- `docs/public/plugins-authoring.md` (reference with migration how-to subsection)
- `docs/plans/plugins/PLUGIN-API.md`
- `apps/web/AGENTS.md`
- `docs/plans/plugin-action-ux/` results and adoption map
- `docs/specs/plugins/{requirements,system-design}/plugin-action-ux.md`
- SDK consumer tests when adding executable documentation examples

## Dependencies

04-compatibility.

## Risks

Do not publish planned API signatures as shipped before implementation. Preserve existing recipe and legacy compatibility guidance.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md)
- [System design](../../specs/plugins/system-design/plugin-action-ux.md)
- [Plan and source audit](plan.md)
- Existing plugin-slot, SDK consumer, and packaged plugin E2E patterns.

## Results

Complete. The public authoring guide, PLUGIN-API, web guidance, and adoption map
now agree with the implemented additive `Action`/`ActionGroup` contract. They
document supported slots, host-selected geometry, legacy slots and Button
compatibility, one-path old-host fallback, composer locations, phone surfaces,
and the compact tablet status-bar exception. Adoption notes name each official
plugin's current migration needs and state the release-evidence limits.

Validation passed:

- Plugin SDK tests (2) and typecheck.
- Focused SDK/action compatibility and renderer tests: 3 files, 14 tests.
- Web typecheck and `pnpm run i18n:check`.
- Focused ESLint on changed action/surface files, with no warnings or errors.
- Public docs tests: 62 passed; all 47 public pages validated.
- Document catalog: 306 decisions and 1153 specifications validated.
- Specification linter: 36 tests passed; all specification files passed.
- `git diff --check`.
