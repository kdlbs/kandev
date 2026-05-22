---
name: mobile-parity
description: Ensures UI feature work ships with desktop and mobile parity, responsive behavior, and mobile Playwright E2E coverage. Use when implementing, planning, reviewing, or testing any new feature, page, component, workflow, form, dialog, sidebar, navigation, dashboard, or visual UI change; if work touches frontend or user-facing UI, this skill must run even when user mentions only desktop or says "new feature".
---

# Mobile Parity

Use this skill before changing UI code for a feature. Goal: feature works on desktop and mobile, and tests prove the mobile path.

## When It Applies

Apply when task changes user-facing UI:

- new or changed pages, routes, components, forms, dialogs, drawers, navigation, dashboards, tables, cards, toolbars, editors, settings, onboarding, or visual states
- new frontend behavior attached to backend/API work
- bug fixes where layout, touch behavior, scrolling, or viewport width can affect success

If task has no UI surface, say why this skill does not apply and continue.

## Workflow

1. Map affected surfaces.
   - Identify every page, modal, menu, tab, empty state, loading state, and error state the feature touches.
   - Check where desktop layout assumptions can fail: fixed widths, hover-only controls, sidebars, tables, dense toolbars, keyboard shortcuts, overflow, and absolute positioning.

2. Design desktop and mobile behavior together.
   - Prefer existing responsive patterns in the repo.
   - Define mobile navigation, stacking order, scrolling region, touch targets, and truncated text behavior before coding.
   - Make primary actions reachable without hover and without horizontal page scroll.

3. Implement responsive UI.
   - Use semantic controls and existing design-system components.
   - Keep touch targets large enough for touch use, generally at least 44px in the active dimension.
   - Ensure forms, dialogs, sheets, menus, tables, and empty states fit narrow screens.
   - Avoid hiding required functionality on mobile unless there is a clear alternate path.

4. Add E2E coverage.
   - Add or update Playwright tests for the feature's happy path on desktop if missing.
   - Add mobile Playwright coverage for the same user value, using existing mobile projects/devices when configured.
   - If no mobile project exists, configure the test or project with a realistic mobile viewport plus touch/mobile settings.
   - Cover mobile-specific controls such as drawers, overflow menus, stacked actions, responsive tables, or bottom controls.

5. Verify visually and behaviorally.
   - Run the narrowest relevant viewport locally or with screenshots when possible.
   - Check that text does not overlap, controls remain clickable, focus/keyboard flows still work, and no unintended horizontal scroll appears.
   - Run the focused Playwright tests. If full E2E cannot run, report the command and blocker.

## Mobile E2E Expectations

Every UI feature should end with one of these:

- mobile Playwright test added or updated
- existing mobile Playwright test explicitly identified as covering the changed behavior
- written justification for no mobile test, limited to non-UI work or impossible-to-test infrastructure gaps

Good mobile tests assert real user outcomes, not only visibility. Prefer:

- open feature from mobile navigation and complete primary action
- use drawer/menu/sheet variant of desktop controls
- submit form and verify result
- handle empty/error/loading state on narrow viewport
- confirm no required action is desktop-only

## Playwright Pattern

Use repo conventions first. When no convention exists, adapt this shape:

```ts
import { test, expect, devices } from '@playwright/test';

test.describe('feature on mobile', () => {
  test.use({ ...devices['iPhone 13'] });

  test('completes primary workflow', async ({ page }) => {
    await page.goto('/feature');
    await page.getByRole('button', { name: /menu|open/i }).click();
    await page.getByRole('link', { name: /feature/i }).click();

    await expect(page.getByRole('heading', { name: /feature/i })).toBeVisible();
    await page.getByRole('button', { name: /primary action/i }).click();
    await expect(page.getByText(/success|saved|created/i)).toBeVisible();
  });
});
```

## Done Checklist

- Desktop path still works.
- Mobile path has designed layout and interaction behavior.
- Required controls are reachable by touch.
- No required workflow depends on hover, wide viewport, or hidden desktop-only UI.
- Mobile Playwright coverage exists or absence is justified.
- Focused tests were run, or exact blocker is reported.
