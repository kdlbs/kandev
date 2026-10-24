---
id: "01-correct-settings-spacing"
title: "Correct settings spacing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-004
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.6
  - AC-UI-SETTINGS-COMPOSITION-002.7
  - AC-UI-SETTINGS-COMPOSITION-002.8
  - AC-UI-SETTINGS-COMPOSITION-004.1
  - AC-UI-SETTINGS-COMPOSITION-004.2
  - AC-UI-SETTINGS-COMPOSITION-004.3
  - AC-UI-SETTINGS-COMPOSITION-004.4
  - AC-UI-SETTINGS-COMPOSITION-004.5
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 01: Correct settings spacing

## Summary

Remove duplicated group padding, bound Task behavior width, and place profile choices below their label.
Use the existing settings components and preserve their state ownership.

## In scope

- The three production components and focused rendered tests listed below.
- Desktop, phone, coarse-pointer, translation, and settings-state checks in the plan.

## Out of scope

- Backend, new copy, global Card or tab styling, and unrelated form redesigns.

## Acceptance

1. Framed groups have one padding owner and 16px first/last content insets, excluding borders, with root-font scaling.
2. Task behavior uses at most 64rem. Profile choices follow the label in two desktop columns and one phone column.
3. All three tabs, translations, keyboard selection, touch targets, help, and Save/Reset retain their existing behavior.

## UI preview

See the [complete preview and mobile contract](plan.md#ui-preview).

| View | Required structure |
| --- | --- |
| UI-01: Desktop Tasks | Bounded title/tabs header, divided setting rows, profile label above two equal choices, then Archiving. |
| UI-02: Phone Tasks | Title, tabs, settings, profile label, stacked choices, and existing draft actions in one vertical flow. |

These views cover the acceptance criteria declared in this work order.

## Implementation sequence

1. Extend the listed browser tests before editing production components.
   Name geometry cases `uses one group padding layer`, `bounds task behavior content`, and `places profile choices below their label`.
   Record failures from excess insets, unbounded content, and incorrect profile geometry, not missing selectors.
2. Apply the class changes specified in the system design. Add an optional class name only to the private radio group.
3. Run focused checks and capture desktop/phone screenshots. Record actual results and update both delivery records.

## Verification

Run from the repository root. Install dependencies once if this worktree has no installation.
Run E2E commands sequentially. The managed runner rebuilds production assets and tears down its isolated runtime.
This NixOS host requires its browser library directories in `LD_LIBRARY_PATH` and `KANDEV_SERVER_HOST=127.0.0.1` for isolated tests.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/settings-group.test.tsx components/settings/task-behavior-settings.test.tsx components/settings/mcp-task-agent-profile-default-settings.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/settings-group.tsx components/settings/task-behavior-settings.tsx components/settings/mcp-task-agent-profile-default-settings.tsx e2e/tests/settings/settings-composition.spec.ts e2e/tests/settings/task-behavior-concise-help.spec.ts e2e/tests/settings/mobile-settings-composition.spec.ts e2e/tests/settings/mobile-task-behavior-concise-help.spec.ts e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts --max-warnings 0)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/settings-composition.spec.ts tests/settings/task-behavior-concise-help.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-settings-composition.spec.ts tests/settings/mobile-task-behavior-concise-help.spec.ts tests/task/mobile-mcp-task-agent-profile-default.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/settings/settings-group.tsx`
- `apps/web/components/settings/task-behavior-settings.tsx`
- `apps/web/components/settings/mcp-task-agent-profile-default-settings.tsx`
- The five browser spec files named in the verification commands.
- This work order and `plan.md` for results.

## Dependencies

None. Implementation follows the user's later explicit instruction.

## Risks

Shared group padding changes require representative neighboring-page checks.
Verify both computed styles and rendered bounds around 768px.

## Parallelism

`sequential`

## Inputs

- [Settings composition requirements](../../specs/ui/requirements/settings-composition.md).
- [Settings composition design](../../specs/ui/system-design/settings-composition.md).
- Existing component tests and the browser specs named above.
- `apps/web/AGENTS.md`, `/mobile-parity`, and `/e2e`.

## Results

The pre-fix browser run failed four geometry cases: duplicate padding, unbounded width, and profile placement at 1920px and 768px.
The 767px stacked profile case already passed.
The initial runner attempts could not start because of a host bind setting and missing Chromium library paths.
Loopback binding and the existing Nix library paths resolved those environment issues.

The three focused component suites passed: 14 tests.
The scoped ESLint command passed.
The specification catalog, specification lint, and whitespace checks passed.
The final desktop E2E command passed all 17 tests.
The final mobile E2E command passed all 12 tests. Two temporary capture cases also passed.
Both runs used `--host`. The final repeat and mobile runs reused the freshly built, unchanged production assets with `--no-build`.
The local PR-documentation coverage preflight passed with status `covered`.
The completed layout matches UI-01 and UI-02, including the phone stack and preserved touch targets.
The final screenshot capture passed both cases. Desktop and phone images were inspected and retained in `apps/web/.pr-assets/task-spacing/`.
The temporary capture spec was removed. The managed runner stopped its isolated backend.

### PR review follow-up

Removed the redundant radio-group gap default and passed the optional class name directly to the shared primitive.
The row caller now declares `grid` explicitly with its responsive columns.
These changes preserve the rendered layout and settings behavior, so the requirements, design, and screenshots remain current.
Verification: the profile component suite passed all 4 tests, scoped ESLint passed, and the Vite production build passed.
The rebuilt frontend passed the 3 profile geometry/keyboard cases with `pnpm e2e:run --host --no-build --project chromium tests/settings/task-behavior-concise-help.spec.ts -- --grep 'places profile choices'`.
It also passed all 5 cases with `pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-task-behavior-concise-help.spec.ts`.
