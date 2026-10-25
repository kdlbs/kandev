---
id: "07-prompt-usage-spacing"
title: "Separate prompt usage help"
status: complete
wave: 7
depends_on:
  - 06-concise-help-and-tabs
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 07: Separate prompt usage help

## Summary

Add the settings group's standard 24px gap between the prompt-reference hint and the Custom prompts heading and action.

## In scope

- Keep the hint and prompt group in their existing page order.
- Apply the shared group spacing on desktop and phone.
- Preserve prompt list, editor, save, and delete behavior.

## Out of scope

- Changing prompt-reference behavior, prompt content, localization, or persistence.
- Rearranging other content on the Prompts settings page.

## Acceptance

- The `@name` hint has the existing 24px settings-group separation from the Custom prompts heading and Add prompt action.
- Desktop and phone retain the existing inline page flow, with the phone group action stacked below its heading.

## ASCII UI preview

UI-08 is the affected region of `/settings/prompts`. The before view is supported by the user-provided desktop screenshot and source; the after views were captured at desktop and phone sizes.

```text
Before, desktop:
[ @name usage hint ]
Custom prompts                                [Add prompt]

Before, phone:
[ @name usage hint ]
Custom prompts
[Add prompt]

After, desktop:
[ @name usage hint ]

Custom prompts                                [Add prompt]
[ prompt list ]

After, phone:
[ @name usage hint ]

Custom prompts
[Add prompt]
[ prompt list ]
```

The page remains the scroll owner. The gap uses the existing `space-y-6` settings spacing token. No new surface, control, or navigation is added. Maps to AC-UI-SETTINGS-COMPOSITION-001.1.

## Verification

- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host tests/settings/prompt-spacing-capture.spec.ts`: passed, one disposable capture scenario; desktop and phone screenshots were inspected and published in PR #3891. The temporary spec was removed after capture.
- Commit hooks: passed formatting, changed-file lint, i18n guards, and commit-message validation.
- `git diff --check`: passed.

## Results

The usage hint and prompt group now have the 24px separation defined by the existing Settings Composition system design. Prompt behavior and persistence are unchanged.
