---
id: "01-anchored-popover"
title: "Render the managed runtime catalogue in an anchored popover"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/runtime-updates.md"
---

# Task 01: Render the managed runtime catalogue in an anchored popover

## Acceptance

- The version trigger opens the searchable catalogue as a popover instead of
  inserting the catalogue into dialog/drawer layout flow.
- Opening the catalogue does not increase the desktop dialog or mobile drawer
  height.
- Desktop and mobile preserve searchable selection, preview callbacks, and
  44px touch-safe option rows.
- Selecting a version closes the popover.

## TDD sequence

1. [x] Add desktop and mobile geometry assertions and confirm they fail against
   the current inline browser.
2. [x] Wrap the trigger and browser with `@kandev/ui/popover`.
3. [x] Rerun the focused tests and refactor only after the behavior is green.

## Validation

Run the commands in `plan.md`, plus `git diff --check` and changed-file ESLint.
